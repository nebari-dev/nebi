package local

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"gorm.io/gorm"
)

// DeleteProjectOptions controls DeleteProject.
type DeleteProjectOptions struct {
	// DeleteFiles removes Path from disk. Only allowed when ManagedDir.
	DeleteFiles bool
}

// pendingRemoval records a project directory that DeleteProject moves
// aside to remove it. It is written before the move and deleted once the
// moved directory is gone, so a failure or crash at any step in between
// leaves a record that RecoverDeletes can finish or undo.
type pendingRemoval struct {
	ID        uuid.UUID `gorm:"type:text;primaryKey"`
	ProjectID uuid.UUID `gorm:"type:text;not null;index"`
	// Path is the project directory before the move.
	Path string `gorm:"not null"`
	// TrashPath is the hidden sibling Path is moved to.
	TrashPath string `gorm:"not null"`
	CreatedAt time.Time
}

func (pendingRemoval) TableName() string { return "client_pending_removals" }

// DeleteProject deletes a local project.
//
//  1. It refuses with ErrFilesNotManaged when DeleteFiles is asked for a
//     directory nebi did not create, and with a *BusyError (errors.Is
//     ErrProjectBusy) while a job for the project is queued or running.
//     The busy check and refusing new jobs for the project are one step in
//     the JobRunner, so nothing can start in between; the check covers
//     only jobs of that runner. It then moves back a directory that an
//     earlier failed delete left moved aside, and refuses with
//     ErrPathTaken when DeleteFiles would remove another project's
//     directory. Apart from that move back, nothing is changed in these
//     cases.
//  2. In one transaction it deletes the project's versions and tags, the
//     rows of the DeleteHooks, its legacy origin row, and the project row.
//  3. With DeleteFiles it first renames Path to a hidden sibling, so the
//     path is free as soon as DeleteProject returns, then submits a job
//     that removes the renamed directory and returns that job; otherwise
//     the files stay and the returned job is nil. The removal is recorded
//     in the database before the rename, so if it cannot finish (the job
//     cannot be submitted, or the process stops) RecoverDeletes completes
//     it later.
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
	// Check for active jobs and stop new ones in one step, so no job can
	// start between the check and the delete. Only the file removal job
	// below is still accepted.
	active, err := s.jobs.RetireProject(ctx, id, jobs.TypeDeleteFiles)
	if err != nil {
		return nil, fmt.Errorf("checking jobs of %s: %w", p.Name, err)
	}
	if len(active) > 0 {
		return nil, &BusyError{ProjectID: id, JobIDs: active}
	}
	restore := func() {
		_ = s.jobs.RestoreProject(context.WithoutCancel(ctx), id)
	}

	// An earlier delete of this project that failed may have left its
	// directory moved aside; put it back so it is kept or removed below
	// like the rest of the project. This runs before the nested-project
	// check, which looks at the directory on disk and would find nothing
	// inside it while it is moved aside.
	if err := s.undoRemovals(ctx, id); err != nil {
		restore()
		return nil, err
	}

	if opts.DeleteFiles {
		if err := s.checkNothingInside(ctx, p); err != nil {
			restore()
			return nil, err
		}
	}

	// Move the directory aside before the row goes, so its path is free
	// at once: a project created at the same path while the removal job
	// runs cannot have its files removed by it.
	var removal *pendingRemoval
	if opts.DeleteFiles {
		if removal, err = s.moveAside(ctx, p); err != nil {
			restore()
			return nil, err
		}
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
		if err := tx.Delete(&LegacyProjectOrigin{}, "project_id = ?", id).Error; err != nil {
			return fmt.Errorf("deleting legacy origin: %w", err)
		}
		if err := tx.Delete(&Project{}, "id = ?", id).Error; err != nil {
			return fmt.Errorf("deleting project row: %w", err)
		}
		return nil
	})
	if err != nil {
		restore()
		if removal != nil {
			if rbErr := s.undoRemoval(context.WithoutCancel(ctx), *removal); rbErr != nil {
				return nil, fmt.Errorf("deleting project %s: %w (and %v; RecoverDeletes retries it)", p.Name, err, rbErr)
			}
		}
		return nil, fmt.Errorf("deleting project %s: %w", p.Name, err)
	}
	s.sizes.forget(id)

	if !opts.DeleteFiles {
		return nil, nil
	}
	if removal == nil {
		// The directory was already gone; the job has nothing to do.
		job, err := s.jobs.Submit(ctx, id, jobs.TypeDeleteFiles, func(context.Context) error { return nil })
		if err != nil {
			return nil, fmt.Errorf("project %s was deleted, but its file removal job could not be started: %w", p.Name, err)
		}
		return job, nil
	}
	job, err := s.jobs.Submit(ctx, id, jobs.TypeDeleteFiles, s.removalJob(*removal))
	if err != nil {
		return nil, fmt.Errorf("project %s was deleted, but removing %s could not be started: %w; RecoverDeletes removes it later", p.Name, removal.TrashPath, err)
	}
	return job, nil
}

// checkNothingInside returns ErrPathTaken when removing p's directory would
// remove another tracked project's directory: the same directory, or one
// inside it. Create refuses nested projects, but rows from the legacy
// migration can still nest, and a directory can be moved under another
// one after it was tracked.
func (s *Service) checkNothingInside(ctx context.Context, p *Project) error {
	tracked, err := s.trackedPaths(ctx)
	if err != nil {
		return err
	}
	for _, o := range tracked {
		if o.ID == p.ID {
			continue
		}
		if sameDir(p.Path, o.Path) {
			return fmt.Errorf("%w: cannot remove %s, project %q at %s uses the same directory", ErrPathTaken, p.Path, o.Name, o.Path)
		}
		if inside(p.Path, o.Path) {
			return fmt.Errorf("%w: cannot remove %s, project %q at %s is inside it", ErrPathTaken, p.Path, o.Name, o.Path)
		}
	}
	return nil
}

// RecoverDeletes finishes or undoes the file removals DeleteProject left
// incomplete, because the removal job could not be submitted or the
// process stopped before the moved-aside directory was gone. Call it once
// when the client starts.
//
// When the project row is gone, the delete went through: it submits a job
// that removes the moved-aside directory, and returns those jobs. When the
// row is still there, the delete did not happen: it moves the directory
// back to the project path. Removals whose job is still running in this
// service's JobRunner are left alone. Like the busy check, this only knows
// the jobs of its own runner, so another client process must not be
// deleting projects in the same database while it runs.
func (s *Service) RecoverDeletes(ctx context.Context) ([]*jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var pending []pendingRemoval
	if err := s.db.WithContext(ctx).Order("created_at").Find(&pending).Error; err != nil {
		return nil, fmt.Errorf("reading pending removals: %w", err)
	}
	var started []*jobs.Job
	var errs []error
	for _, r := range pending {
		running, err := s.removalRunning(ctx, r.ProjectID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if running {
			continue
		}
		var n int64
		if err := s.db.WithContext(ctx).Model(&Project{}).Where("id = ?", r.ProjectID).Count(&n).Error; err != nil {
			errs = append(errs, fmt.Errorf("checking project %s: %w", r.ProjectID, err))
			continue
		}
		if n > 0 {
			if err := s.undoRemoval(ctx, r); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		job, err := s.jobs.Submit(ctx, r.ProjectID, jobs.TypeDeleteFiles, s.removalJob(r))
		if err != nil {
			errs = append(errs, fmt.Errorf("starting removal of %s: %w", r.TrashPath, err))
			continue
		}
		started = append(started, job)
	}
	return started, errors.Join(errs...)
}

// moveAside records the removal of p's directory and renames the directory
// to a hidden sibling named after the project ID. The sibling is on the
// same filesystem, so the rename is atomic. It returns nil when the
// directory does not exist.
func (s *Service) moveAside(ctx context.Context, p *Project) (*pendingRemoval, error) {
	r := &pendingRemoval{
		ID:        uuid.New(),
		ProjectID: p.ID,
		Path:      p.Path,
		TrashPath: filepath.Join(filepath.Dir(p.Path), "."+filepath.Base(p.Path)+".deleting-"+p.ID.String()),
	}
	if err := s.db.WithContext(ctx).Create(r).Error; err != nil {
		return nil, fmt.Errorf("recording removal of %s: %w", p.Path, err)
	}
	if err := os.Rename(r.Path, r.TrashPath); err != nil {
		// Nothing moved, so the record is not needed. If dropping it fails,
		// RecoverDeletes drops it.
		_ = s.dropRemoval(context.WithoutCancel(ctx), r.ID)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("moving %s aside for removal: %w", r.Path, err)
	}
	return r, nil
}

// undoRemovals moves back every directory of the project that a failed
// delete left moved aside.
func (s *Service) undoRemovals(ctx context.Context, projectID uuid.UUID) error {
	var pending []pendingRemoval
	if err := s.db.WithContext(ctx).Where("project_id = ?", projectID).Order("created_at").Find(&pending).Error; err != nil {
		return fmt.Errorf("reading pending removals: %w", err)
	}
	for _, r := range pending {
		if err := s.undoRemoval(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

// undoRemoval moves a moved-aside directory back to its project path, for
// a delete that did not happen, and drops the record. It refuses when
// something else is at the project path now.
func (s *Service) undoRemoval(ctx context.Context, r pendingRemoval) error {
	if _, err := os.Lstat(r.TrashPath); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("checking %s: %w", r.TrashPath, err)
		}
		// Never moved, or moved back already.
		return s.dropRemoval(ctx, r.ID)
	}
	if _, err := os.Lstat(r.Path); err == nil {
		return fmt.Errorf("moving %s back failed: %s exists", r.TrashPath, r.Path)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("checking %s: %w", r.Path, err)
	}
	if err := os.Rename(r.TrashPath, r.Path); err != nil {
		return fmt.Errorf("moving %s back to %s failed: %w", r.TrashPath, r.Path, err)
	}
	return s.dropRemoval(ctx, r.ID)
}

// removalJob is the body of the job that removes a moved-aside directory.
// The record is dropped only once the directory is gone, so a failed or
// interrupted removal is retried by RecoverDeletes.
func (s *Service) removalJob(r pendingRemoval) jobs.Func {
	return func(ctx context.Context) error {
		if err := removeProjectDir(r.TrashPath); err != nil {
			return err
		}
		return s.dropRemoval(context.WithoutCancel(ctx), r.ID)
	}
}

// removalRunning reports whether a file removal job of the project is
// queued or running.
func (s *Service) removalRunning(ctx context.Context, projectID uuid.UUID) (bool, error) {
	list, err := s.jobs.ProjectJobs(ctx, projectID)
	if err != nil {
		return false, fmt.Errorf("reading jobs of project %s: %w", projectID, err)
	}
	for _, j := range list {
		if j.Type == jobs.TypeDeleteFiles && j.Active() {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) dropRemoval(ctx context.Context, id uuid.UUID) error {
	if err := s.db.WithContext(ctx).Delete(&pendingRemoval{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("clearing removal record: %w", err)
	}
	return nil
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
