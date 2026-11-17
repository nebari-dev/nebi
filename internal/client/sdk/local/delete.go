package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"gorm.io/gorm"
)

// DeleteProjectOptions controls DeleteProject.
type DeleteProjectOptions struct {
	// DeleteFiles removes Path from disk. Only allowed when ManagedDir.
	DeleteFiles bool
}

// DeleteProject deletes a local project.
//
//  1. It refuses with a *BusyError (errors.Is ErrProjectBusy) while a job
//     for the project is queued or running, and with ErrFilesNotManaged
//     when DeleteFiles is asked for a directory nebi did not create, and
//     with ErrPathTaken when DeleteFiles would remove another project's
//     directory. Nothing is changed in these cases.
//  2. In one transaction it deletes the project's versions and tags, the
//     rows of the DeleteHooks, and the project row.
//  3. With DeleteFiles it submits a job that removes Path and returns that
//     job; otherwise the files stay and the returned job is nil.
//
// The server is never contacted: a project that was pushed stays on the
// server and can be pulled again.
func (s *Service) DeleteProject(ctx context.Context, id uuid.UUID, opts DeleteProjectOptions) (*jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.loadProject(ctx, id)
	if err != nil {
		return nil, err
	}
	if opts.DeleteFiles && !p.ManagedDir {
		return nil, fmt.Errorf("%w: %s; remove it yourself if you no longer need it", ErrFilesNotManaged, p.Path)
	}
	if opts.DeleteFiles {
		// Create refuses nested projects, but rows from the legacy
		// migration can still nest; never remove another project's files.
		tracked, err := s.trackedPaths(ctx)
		if err != nil {
			return nil, err
		}
		for _, o := range tracked {
			if o.ID != id && isWithin(p.Path, o.Path) {
				return nil, fmt.Errorf("%w: cannot remove %s, project %q at %s is inside it", ErrPathTaken, p.Path, o.Name, o.Path)
			}
		}
	}
	projectJobs, err := s.jobs.ProjectJobs(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reading jobs of %s: %w", p.Name, err)
	}
	var active []uuid.UUID
	for _, j := range projectJobs {
		if j.Active() {
			active = append(active, j.ID)
		}
	}
	if len(active) > 0 {
		return nil, &BusyError{ProjectID: id, JobIDs: active}
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.versions.DeleteProjectVersions(tx, id); err != nil {
			return fmt.Errorf("deleting versions: %w", err)
		}
		for _, hook := range s.deleteHooks {
			if err := hook(tx, id); err != nil {
				return err
			}
		}
		if err := tx.Delete(&Project{}, "id = ?", id).Error; err != nil {
			return fmt.Errorf("deleting project row: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("deleting project %s: %w", p.Name, err)
	}
	s.sizes.forget(id)

	if !opts.DeleteFiles {
		return nil, nil
	}
	dir := p.Path
	job, err := s.jobs.Submit(ctx, id, jobs.TypeDeleteFiles, func(context.Context) error {
		return removeProjectDir(dir)
	})
	if err != nil {
		return nil, fmt.Errorf("project %s was deleted, but removing %s could not be started: %w", p.Name, dir, err)
	}
	return job, nil
}

// removeProjectDir removes a managed project directory. It refuses paths
// that cannot be a project directory, as a last guard against wiping the
// wrong place.
func removeProjectDir(dir string) error {
	if !filepath.IsAbs(dir) || filepath.Dir(dir) == dir {
		return fmt.Errorf("refusing to remove %q", dir)
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == filepath.Clean(dir) {
		return fmt.Errorf("refusing to remove the home directory %q", dir)
	}
	return os.RemoveAll(dir)
}
