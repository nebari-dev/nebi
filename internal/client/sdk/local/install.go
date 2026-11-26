package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/jobs"
)

// InstallRequest describes the install of a project version.
type InstallRequest struct {
	ProjectID uuid.UUID
	// VersionID is the version to install. Nil means the last installed
	// version, or the latest version when nothing was installed before.
	VersionID *uuid.UUID
	// Force installs even when the working copy has changes that are not
	// in any recorded version. Those changes are overwritten.
	Force bool
}

// InstallVersion installs a version of a project into its directory and
// returns the install job. One version is installed at a time: installing
// another replaces it. Installing the version that is already installed
// runs pixi again, which repairs a damaged environment.
//
//  1. It resolves the version: req.VersionID, or else the last installed
//     version, or else the latest. It returns ErrNotFound for an unknown
//     project or version, and ErrNoVersions when no version is given and
//     the project has none.
//  2. It refuses with a *BusyError (errors.Is ErrProjectBusy) while a job
//     for the project is pending or running, so a second click cannot
//     queue a duplicate install.
//  3. The install overwrites pixi.toml and pixi.lock in the project
//     directory, so unless req.Force is set it refuses with
//     ErrWorkingCopyModified when they hold changes that are in no recorded
//     version (record them with CreateResolvedVersion first). It returns
//     ErrWorkingCopyMissing when the project directory is gone.
//  4. It submits an install job that runs installStep, the step the create
//     and create_version jobs share: it clears InstalledVersionID, writes
//     the version's files, runs pixi install with pixi's output in the
//     job's log, and on success sets InstalledVersionID and
//     LastInstalledVersionID.
//
// A failed or cancelled install leaves InstalledVersionID nil, the target
// version's files in place and LastInstalledVersionID unchanged; the state
// is then StateInstallFailed, linked to the job. The previous version is
// not put back: the user installs it again, which pixi's cache makes fast.
func (s *Service) InstallVersion(ctx context.Context, req InstallRequest) (*jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.loadProject(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}
	v, err := s.versionToInstall(ctx, p, req.VersionID)
	if err != nil {
		return nil, err
	}
	// The checks and the Submit run under s.mu, which every other
	// operation that submits a job for a project also holds, so no job can
	// be submitted for the project in between.
	if err := s.checkIdle(ctx, p.ID); err != nil {
		return nil, err
	}
	if err := s.checkInstallable(ctx, p.ID, p.Path, req.Force); err != nil {
		return nil, err
	}
	force := req.Force
	job, err := s.jobs.Submit(ctx, p.ID, jobs.TypeInstall, func(ctx context.Context, log io.Writer) error {
		return s.installStep(ctx, p.ID, p.Path, v, force, log)
	})
	if err != nil {
		return nil, fmt.Errorf("submitting install job: %w", err)
	}
	return job, nil
}

// versionToInstall returns the whole version InstallVersion installs: the
// one with ID id, or when id is nil the last installed version, or the
// latest one when none was installed.
func (s *Service) versionToInstall(ctx context.Context, p *Project, id *uuid.UUID) (*ProjectVersion, error) {
	db := s.db.WithContext(ctx)
	if id != nil {
		return versionByID(db, p.ID, *id)
	}
	if p.LastInstalledVersionID != nil {
		v, err := versionByID(db, p.ID, *p.LastInstalledVersionID)
		// Versions are only deleted with their project, so the last
		// installed one is always there; fall back to the latest anyway.
		if !errors.Is(err, ErrNotFound) {
			return v, err
		}
	}
	v, err := latestVersion(db, p.ID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, fmt.Errorf("%w: project %s has nothing to install", ErrNoVersions, p.Name)
	}
	return v, nil
}

// Uninstall removes a project's environment and returns the uninstall job.
//
// It refuses with a *BusyError (errors.Is ErrProjectBusy) while a job for
// the project is pending or running, and returns ErrNotInstalled when
// there is nothing to remove: no InstalledVersionID and no .pixi/envs. An
// environment left behind by a failed install, which has no
// InstalledVersionID, is removed.
//
// The job clears InstalledVersionID, then removes .pixi/envs. pixi.toml,
// pixi.lock and LastInstalledVersionID are kept, so the project can be
// installed again.
func (s *Service) Uninstall(ctx context.Context, projectID uuid.UUID) (*jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.loadProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if err := s.checkIdle(ctx, p.ID); err != nil {
		return nil, err
	}
	envs := envsDir(p.Path)
	if p.InstalledVersionID == nil {
		switch _, err := os.Lstat(envs); {
		case errors.Is(err, fs.ErrNotExist):
			return nil, fmt.Errorf("%w: %s has no installed version and no %s", ErrNotInstalled, p.Name, envs)
		case err != nil:
			return nil, fmt.Errorf("checking %s: %w", envs, err)
		}
	}
	job, err := s.jobs.Submit(ctx, p.ID, jobs.TypeUninstall, s.uninstallJob(p.ID, envs))
	if err != nil {
		return nil, fmt.Errorf("submitting uninstall job: %w", err)
	}
	return job, nil
}

// uninstallJob is the body of the uninstall job. It clears
// InstalledVersionID before touching .pixi, so a crash or failure midway
// does not leave the project reported as installed, then removes envs.
// A removal that fails part way leaves the project not installed; running
// Uninstall again removes the rest.
func (s *Service) uninstallJob(projectID uuid.UUID, envs string) jobs.Func {
	return func(ctx context.Context, log io.Writer) error {
		if err := s.db.WithContext(ctx).Model(&Project{}).Where("id = ?", projectID).
			Update("installed_version_id", nil).Error; err != nil {
			return fmt.Errorf("clearing the installed version: %w", err)
		}
		fmt.Fprintf(log, "removing %s\n", envs)
		// RemoveAll removes a symlink at envs, never what it points to.
		if err := os.RemoveAll(envs); err != nil {
			return fmt.Errorf("removing %s: %w", envs, err)
		}
		return nil
	}
}

// envsDir is where pixi installs a project's environments.
func envsDir(projectDir string) string {
	return filepath.Join(projectDir, ".pixi", "envs")
}
