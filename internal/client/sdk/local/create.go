package local

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/jobs"
)

// Stages the create job reports through jobs.ReportStage. A failed create
// job whose stage is StageInstall solved its lock file and recorded version
// 1 before the install failed.
const (
	StageLock    = "lock"
	StageInstall = "install"
)

// CreateProjectRequest describes a new project.
type CreateProjectRequest struct {
	Name string
	// Path is the project directory. Empty means DefaultProjectPath(Name),
	// and the directory is created and marked ManagedDir.
	Path string

	// Exactly one of Spec (structured input) or Manifest (raw pixi.toml) is set.
	Spec     *artifact.Spec
	Manifest string

	// Description is written to [workspace].description in pixi.toml.
	Description string
	// Tags are applied to version 1 when it is recorded.
	Tags []string
	// Install creates and installs in one step: after the lock is solved
	// and version 1 is recorded, the create job runs `pixi install` and sets
	// InstalledVersionID. Solving first means a failed install still leaves
	// version 1 behind, and the state tells the two failures apart.
	Install bool
}

// CreateProject validates req, writes the project directory and pixi.toml,
// inserts the project row and submits a create job, and returns the project
// and that job.
//
// The job solves the lock file and records version 1 (origin "created", no
// base, req.Tags). With req.Install it then installs the environment and
// sets InstalledVersionID. When the job fails the project is kept, with
// pixi.toml as written, so the user can fix it or delete the project: with
// no versions its state is StateLockFailed, linked to the job.
//
// ManagedDir is set only when CreateProject itself creates the directory
// and the directory is inside the projects directory. An existing
// directory, including one at the default location, is never managed, so
// DeleteProject never removes files nebi did not create.
func (s *Service) CreateProject(ctx context.Context, req CreateProjectRequest) (*Project, *jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Validate the request before touching anything.
	if err := s.ValidateProjectName(ctx, req.Name, nil); err != nil {
		return nil, nil, err
	}
	manifest, err := buildManifest(req)
	if err != nil {
		return nil, nil, err
	}
	dir, err := s.createPath(req)
	if err != nil {
		return nil, nil, err
	}
	if err := s.checkPathFree(ctx, dir); err != nil {
		return nil, nil, err
	}
	projectsDir, err := s.resolvedProjectsDir()
	if err != nil {
		return nil, nil, err
	}

	// 2. Create the directory and pixi.toml.
	created, err := ensureDir(dir)
	if err != nil {
		return nil, nil, err
	}
	manifestPath := filepath.Join(dir, artifact.ManifestFile)
	cleanup := func() {
		if created {
			_ = os.RemoveAll(dir)
		} else {
			_ = os.Remove(manifestPath)
		}
	}
	if err := writeNewFile(manifestPath, manifest); err != nil {
		if created {
			_ = os.RemoveAll(dir)
		}
		if errors.Is(err, fs.ErrExist) {
			return nil, nil, fmt.Errorf("%w: %s", ErrPathNotEmpty, dir)
		}
		return nil, nil, fmt.Errorf("writing %s: %w", manifestPath, err)
	}

	// 3. Insert the project row.
	p := &Project{
		Name:       req.Name,
		Path:       dir,
		ManagedDir: created && isWithin(projectsDir, dir),
	}
	if err := s.db.WithContext(ctx).Create(p).Error; err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("creating project: %w", uniqueViolation(err, p))
	}

	// 4. Submit the create job.
	job, err := s.jobs.Submit(ctx, p.ID, jobs.TypeCreate, s.createJob(p.ID, dir, cleanTags(req.Tags), req.Install))
	if err != nil {
		// Nothing ran: undo the row and the files so the name and the
		// directory are free for a retry.
		if delErr := s.db.WithContext(context.WithoutCancel(ctx)).Delete(&Project{}, "id = ?", p.ID).Error; delErr != nil {
			return nil, nil, fmt.Errorf("submitting create job: %w (and removing the project row failed: %v)", err, delErr)
		}
		cleanup()
		return nil, nil, fmt.Errorf("submitting create job: %w", err)
	}
	return p, job, nil
}

// buildManifest checks that exactly one of Spec and Manifest is set and
// that there is at least one platform, and returns the pixi.toml to write
// with [workspace].name (and description, when given) set.
func buildManifest(req CreateProjectRequest) (string, error) {
	hasSpec := req.Spec != nil
	hasManifest := strings.TrimSpace(req.Manifest) != ""
	if hasSpec == hasManifest {
		return "", fmt.Errorf("%w: set exactly one of Spec or Manifest", ErrInvalidManifest)
	}

	if hasSpec {
		if !hasPlatform(req.Spec.Platforms) {
			return "", ErrNoPlatform
		}
		out, err := artifact.RenderManifest(req.Name, req.Description, *req.Spec)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidManifest, err)
		}
		return out, nil
	}

	parsed, err := artifact.ParseManifest(req.Manifest)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if !hasPlatform(parsed.Platforms) {
		return "", ErrNoPlatform
	}
	out, err := artifact.SetWorkspaceFields(req.Manifest, req.Name, req.Description)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	return out, nil
}

func hasPlatform(platforms []string) bool {
	for _, p := range platforms {
		if strings.TrimSpace(p) != "" {
			return true
		}
	}
	return false
}

// createPath returns the absolute, symlink-resolved project directory.
func (s *Service) createPath(req CreateProjectRequest) (string, error) {
	if req.Path == "" {
		return s.DefaultProjectPath(req.Name)
	}
	return normalizePath(req.Path)
}

// checkPathFree returns ErrPathTaken when a project already tracks dir and
// ErrPathNotEmpty when dir already has a pixi.toml.
func (s *Service) checkPathFree(ctx context.Context, dir string) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&Project{}).Where("path = ?", dir).Count(&count).Error; err != nil {
		return fmt.Errorf("checking project path: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: %s", ErrPathTaken, dir)
	}
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("checking %s: %w", dir, err)
	case !info.IsDir():
		return fmt.Errorf("%s exists and is not a directory", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, artifact.ManifestFile)); err == nil {
		return fmt.Errorf("%w: %s", ErrPathNotEmpty, dir)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("checking %s: %w", dir, err)
	}
	return nil
}

// ensureDir creates dir when it does not exist and reports whether it did.
func ensureDir(dir string) (bool, error) {
	if _, err := os.Stat(dir); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("checking %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("creating %s: %w", dir, err)
	}
	return true, nil
}

// writeNewFile writes content to a file that must not exist yet, so two
// creates racing for one directory cannot overwrite each other's pixi.toml.
func writeNewFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		_ = os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

// cleanTags trims tags and drops empty and repeated ones, keeping order.
func cleanTags(tags []string) []string {
	var out []string
	seen := make(map[string]bool, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// createJob is the body of the create job.
func (s *Service) createJob(projectID uuid.UUID, dir string, tags []string, install bool) jobs.Func {
	return func(ctx context.Context) error {
		jobs.ReportStage(ctx, StageLock)
		if err := s.pixi.Lock(ctx, dir); err != nil {
			return fmt.Errorf("pixi lock: %w", err)
		}
		manifest, err := os.ReadFile(filepath.Join(dir, artifact.ManifestFile))
		if err != nil {
			return fmt.Errorf("reading %s: %w", artifact.ManifestFile, err)
		}
		lock, err := os.ReadFile(filepath.Join(dir, artifact.LockFile))
		if err != nil {
			return fmt.Errorf("reading %s: %w", artifact.LockFile, err)
		}
		in := RecordVersionInput{
			ProjectID: projectID,
			Manifest:  string(manifest),
			Lock:      string(lock),
			Origin:    VersionOriginCreated,
			Tags:      tags,
		}
		if id, ok := jobs.IDFromContext(ctx); ok {
			in.JobID = &id
		}
		versionID, err := s.versions.RecordVersion(ctx, in)
		if err != nil {
			return fmt.Errorf("recording version 1: %w", err)
		}
		if !install {
			return nil
		}

		jobs.ReportStage(ctx, StageInstall)
		if err := s.pixi.Install(ctx, dir); err != nil {
			return fmt.Errorf("pixi install: %w", err)
		}
		if err := s.db.WithContext(ctx).Model(&Project{}).Where("id = ?", projectID).
			Update("installed_version_id", versionID).Error; err != nil {
			return fmt.Errorf("marking version 1 installed: %w", err)
		}
		return nil
	}
}
