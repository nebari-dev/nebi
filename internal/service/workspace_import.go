package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
)

// ImportFromRegistryRequest selects the OCI bundle to import.
type ImportFromRegistryRequest struct {
	// Repository is the existing namespace-relative repository name. It is
	// resolved against the registry's configured namespace.
	Repository string
	// RepositoryPath is the full OCI repository path below the registry host,
	// for example "namespace/repository" in "host/namespace/repository:tag".
	RepositoryPath string
	Tag            string
	Name           string
}

// ImportFromRegistry pulls an OCI bundle from a registered registry and
// creates a new workspace populated from it.
//
// Flow is identical for both modes: resolve the registry, stage every
// fetched artifact under the executor's import-staging root, then enqueue
// a workspace-create job that points the worker at the staging dir
// (CreateWorkspaceOptions.SeedDir). The mode difference is purely how
// many bytes get staged:
//
//   - local mode: oci.ExtractBundle streams pixi.toml + pixi.lock + every
//     asset layer to disk. The worker seeds the workspace from the full
//     bundle, so asset files and the published lockfile both round-trip.
//   - team mode: oci.PullBundle fetches only the two core layers; the
//     handler writes them to the staging dir as plain files. Asset
//     layers are listed in the manifest but not fetched. pixi.lock is
//     still preserved on disk for the worker to use, fixing a latent
//     bug where team-mode imports re-solved from pixi.toml alone.
//
// Network errors surface synchronously so the caller knows the import
// did not start. Failures the caller can act on come back typed (see
// classifyBundlePullError) instead of as an opaque internal error. On
// any failure after the staging dir is created, the staging dir is
// removed before returning.
func (s *WorkspaceService) ImportFromRegistry(ctx context.Context, registryID string, req ImportFromRegistryRequest, userID uuid.UUID) (*models.Workspace, error) {
	regID, err := uuid.Parse(registryID)
	if err != nil {
		return nil, &ValidationError{Message: "Invalid registry ID"}
	}
	if err := ensureRegistryAccess(s.db, s.rbac, s.isLocal, userID, regID, "read"); err != nil {
		return nil, err
	}

	ep, err := s.loadRegistryEndpoint(registryID)
	if err != nil {
		return nil, err
	}
	repository := strings.TrimSpace(req.Repository)
	repositoryPath := strings.TrimSpace(req.RepositoryPath)
	if repository != "" && repositoryPath != "" {
		return nil, &ValidationError{Message: "provide either repository or repository_path, not both"}
	}

	auditRepository := repository
	var repoRef string
	if repositoryPath != "" {
		auditRepository = repositoryPath
		repoRef = ep.RepositoryPathRef(repositoryPath)
	} else if repository != "" {
		repoRef = ep.NamespaceRelativeRepoRef(repository)
	} else {
		return nil, &ValidationError{Message: "repository or repository_path is required"}
	}
	pullOpts := oci.PullOptions{
		Username:  ep.Username,
		Password:  ep.Password,
		PlainHTTP: ep.PlainHTTP,
		// Cap total bundle size to defend against a malicious or
		// misconfigured registry serving a runaway asset layer. 5 GiB
		// is well above any reasonable Pixi environment but small
		// enough that exhausting disk requires deliberate effort.
		MaxBundleBytes: 5 * 1024 * 1024 * 1024,
	}

	// Cap the synchronous OCI pull so a slow or malicious registry
	// cannot hold an HTTP request open indefinitely. Generous enough
	// for legitimate large bundles on slow networks; tight enough that
	// hung requests free up server resources.
	pullCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	stagingDir, err := os.MkdirTemp(s.executor.StagingRoot(), "import-")
	if err != nil {
		return nil, fmt.Errorf("create staging dir: %w", err)
	}

	var digest string
	if s.isLocal {
		result, err := oci.ExtractBundle(pullCtx, repoRef, req.Tag, stagingDir, pullOpts)
		if err != nil {
			_ = os.RemoveAll(stagingDir)
			return nil, classifyBundlePullError("extract bundle", err, repoRef, req.Tag)
		}
		digest = result.Digest
	} else {
		result, err := oci.PullBundle(pullCtx, repoRef, req.Tag, pullOpts)
		if err != nil {
			_ = os.RemoveAll(stagingDir)
			return nil, classifyBundlePullError("pull bundle", err, repoRef, req.Tag)
		}
		// Stage just the two core files; asset layers stay in the
		// registry until team mode opts in to bundle support.
		if err := os.WriteFile(filepath.Join(stagingDir, "pixi.toml"), []byte(result.PixiToml), 0o644); err != nil {
			_ = os.RemoveAll(stagingDir)
			return nil, fmt.Errorf("stage pixi.toml: %w", err)
		}
		if err := os.WriteFile(filepath.Join(stagingDir, "pixi.lock"), []byte(result.PixiLock), 0o644); err != nil {
			_ = os.RemoveAll(stagingDir)
			return nil, fmt.Errorf("stage pixi.lock: %w", err)
		}
		digest = result.Digest
	}

	ws, err := s.Create(ctx, CreateRequest{
		Name:             req.Name,
		ImportStagingDir: stagingDir,
	}, userID)
	if err != nil {
		_ = os.RemoveAll(stagingDir)
		return nil, err
	}

	audit.LogAction(s.db, userID, audit.ActionImportWorkspace, fmt.Sprintf("ws:%s", ws.ID.String()), map[string]interface{}{
		"name":       req.Name,
		"registry":   ep.Registry.Name,
		"repository": auditRepository,
		"tag":        req.Tag,
		"digest":     digest,
	})

	return ws, nil
}

// classifyBundlePullError turns a failed bundle pull into a typed service
// error when the caller can act on it, so API clients can tell a bad
// artifact or a registry refusal from a broken server:
//
//   - the artifact is not a Nebi bundle, or is a malformed one →
//     UnprocessableError carrying the reason;
//   - the registry has no such repository or tag → NotFoundError;
//   - the pull was refused with 401/403 by the registry or something
//     it delegates to (its token service, a host it redirected to) →
//     UpstreamError carrying that status.
//
// Anything else is wrapped with op and stays an internal error.
//
// The typed errors are built only from fixed text, the repository
// reference the caller asked for, and the upstream status. Nothing the
// registry sent (manifest fields, response bodies) and nothing from the
// underlying client error (request URLs, credentials) is copied into
// them, because both the response body and the log are downstream.
func classifyBundlePullError(op string, err error, repoRef, tag string) error {
	if errors.Is(err, oci.ErrNotNebiArtifact) {
		return &UnprocessableError{Message: "not a Nebi artifact"}
	}
	var invalid *oci.InvalidBundleError
	if errors.As(err, &invalid) {
		return &UnprocessableError{Message: "invalid bundle: " + invalid.Reason}
	}
	ref := displayRepoRef(repoRef)
	if errors.Is(err, oci.ErrReferenceNotFound) {
		return &NotFoundError{Message: fmt.Sprintf("repository or tag not found: %s:%s", ref, tag)}
	}
	var refused *oci.RegistryAccessError
	if errors.As(err, &refused) {
		return &UpstreamError{
			Message:        fmt.Sprintf("registry refused access to %s", ref),
			UpstreamStatus: refused.StatusCode,
			Op:             op,
			Target:         ref,
		}
	}
	return fmt.Errorf("%s: %w", op, err)
}

// displayRepoRef returns repoRef in a form safe to show to a caller: any
// userinfo embedded in the host part ("user:secret@host/repo") is dropped.
func displayRepoRef(repoRef string) string {
	host, rest, hasPath := strings.Cut(repoRef, "/")
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	if !hasPath {
		return host
	}
	return host + "/" + rest
}
