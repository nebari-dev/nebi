package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
)

// ImportFromRegistryRequest selects the OCI bundle to import.
type ImportFromRegistryRequest struct {
	Repository string
	Tag        string
	Name       string
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
// did not start. On any failure after the staging dir is created, the
// staging dir is removed before returning.
func (s *WorkspaceService) ImportFromRegistry(ctx context.Context, registryID string, req ImportFromRegistryRequest, userID uuid.UUID) (*models.Workspace, error) {
	ep, err := s.loadRegistryEndpoint(registryID)
	if err != nil {
		return nil, err
	}
	repoRef := ep.RepoRef(req.Repository)
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
			return nil, fmt.Errorf("extract bundle: %w", err)
		}
		digest = result.Digest
	} else {
		result, err := oci.PullBundle(pullCtx, repoRef, req.Tag, pullOpts)
		if err != nil {
			_ = os.RemoveAll(stagingDir)
			return nil, fmt.Errorf("pull bundle: %w", err)
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
		PackageManager:   "pixi",
		ImportStagingDir: stagingDir,
	}, userID)
	if err != nil {
		_ = os.RemoveAll(stagingDir)
		return nil, err
	}

	audit.LogAction(s.db, userID, audit.ActionImportWorkspace, fmt.Sprintf("ws:%s", ws.ID.String()), map[string]interface{}{
		"name":       req.Name,
		"registry":   ep.Registry.Name,
		"repository": req.Repository,
		"tag":        req.Tag,
		"digest":     digest,
	})

	return ws, nil
}
