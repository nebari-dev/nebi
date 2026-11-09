package exec

// Owner: U2 (detail). Install a specific version / last installed / latest,
// uninstall, and the corresponding job bodies.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/utils"
	"gorm.io/gorm"
)

// restoreTimeout bounds the reinstall of the previous version after a
// failed or cancelled install (packages are usually in pixi's cache, so it
// only relinks them).
const restoreTimeout = 15 * time.Minute

// frozenInstaller is implemented by executors that can install exactly
// what pixi.lock records (executor.LocalExecutor: `pixi install --frozen`).
type frozenInstaller interface {
	InstallEnvironmentFrozen(ctx context.Context, project *models.Project, logWriter io.Writer) error
}

// InstallVersion submits a JobTypeInstallVersion job. A nil req.Version
// resolves to the last installed version, else the latest version. The
// resolved number is stored on job.VersionNumber.
func (s *Service) InstallVersion(ctx context.Context, projectID uuid.UUID, req shared.InstallRequest) (*shared.JobSummary, error) {
	project, err := s.project(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if err := s.checkIdle(ctx, project); err != nil {
		return nil, err
	}
	n, err := s.resolveInstallVersion(ctx, project, req.Version)
	if err != nil {
		return nil, err
	}
	if _, err := s.local.LoadVersion(ctx, projectID, n); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, &shared.ValidationError{Field: "version", Message: fmt.Sprintf("Version %d is not on this machine. Pull version %d first.", n, n)}
		}
		return nil, err
	}
	job, err := s.deps.Runtime.Jobs().Submit(ctx, shared.JobRequest{
		ProjectID:     projectID,
		Type:          models.JobTypeInstallVersion,
		VersionNumber: &n,
	})
	return summarize(job, project.Name, err)
}

// Uninstall submits a JobTypeUninstall job for the installed version.
func (s *Service) Uninstall(ctx context.Context, projectID uuid.UUID) (*shared.JobSummary, error) {
	project, err := s.project(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if err := s.checkIdle(ctx, project); err != nil {
		return nil, err
	}
	if project.InstalledVersion == nil && !s.deps.Executor.IsEnvInstalled(project) {
		return nil, &shared.ConflictError{Code: "not_installed", Message: "No version of this project is installed."}
	}
	job, err := s.deps.Runtime.Jobs().Submit(ctx, shared.JobRequest{
		ProjectID:     projectID,
		Type:          models.JobTypeUninstall,
		VersionNumber: project.InstalledVersion,
	})
	return summarize(job, project.Name, err)
}

// RunInstallVersion is the install_version job body: write the version's
// pixi.toml/pixi.lock into the project directory, pixi install, then set
// Project.InstalledVersion and LastInstalledVersion and the size on disk.
//
// The install is frozen (`pixi install --frozen`) whenever the version has
// a lock, so the environment is exactly the stored version and the lock is
// never re-solved. One version is installed at a time: installing N
// replaces whatever is installed. When the install fails or is cancelled
// the previous version's files are put back and, if a version was
// installed, it is reinstalled so the project is never left half
// replaced; if that also fails the project is marked not installed.
//
// Other job bodies (create and install, pull and install) may call this
// with their own job: it only uses job.ProjectID and job.VersionNumber
// (nil means last installed, else latest) and sets job.VersionNumber.
func (s *Service) RunInstallVersion(ctx context.Context, job *models.Job, log io.Writer) error {
	project, err := s.project(ctx, job.ProjectID)
	if err != nil {
		return err
	}
	n, err := s.resolveInstallVersion(ctx, project, job.VersionNumber)
	if err != nil {
		return err
	}
	job.VersionNumber = &n
	version, err := s.local.LoadVersion(ctx, project.ID, n)
	if err != nil {
		return err
	}

	dir := s.deps.Executor.GetProjectPath(project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating project directory: %w", err)
	}
	before := snapshotFiles(dir)
	envExisted := s.deps.Executor.IsEnvInstalled(project)
	previous := project.InstalledVersion
	if previous != nil && *previous != n {
		fmt.Fprintf(log, "Installing version %d of %s (replaces version %d)\n", n, project.Name, *previous)
	} else {
		fmt.Fprintf(log, "Installing version %d of %s\n", n, project.Name)
	}
	fmt.Fprintf(log, "Project directory: %s\n", dir)

	installErr := s.writeAndInstall(ctx, project, dir, version, log)
	if installErr != nil {
		s.rollbackInstall(ctx, project, dir, n, previous, before, envExisted, log)
		if errors.Is(installErr, context.Canceled) || ctx.Err() != nil {
			return installErr
		}
		return fmt.Errorf("version %d couldn't install: %w", n, installErr)
	}

	size := s.envSize(project)
	updates := map[string]any{
		"installed_version":      n,
		"last_installed_version": n,
		"size_bytes":             size,
	}
	if project.Status == models.ProjectStatusFailed {
		updates["status"] = models.ProjectStatusReady
	}
	if err := s.db(ctx).Model(&models.Project{}).Where("id = ?", project.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("recording installed version: %w", err)
	}
	fmt.Fprintf(log, "Version %d installed · ready to use on this machine\n", n)
	return nil
}

// RunUninstall is the uninstall job body: remove .pixi/envs and clear
// Project.InstalledVersion (LastInstalledVersion stays).
func (s *Service) RunUninstall(ctx context.Context, job *models.Job, log io.Writer) error {
	project, err := s.project(ctx, job.ProjectID)
	if err != nil {
		return err
	}
	if project.InstalledVersion != nil {
		job.VersionNumber = project.InstalledVersion
		fmt.Fprintf(log, "Uninstalling version %d of %s\n", *project.InstalledVersion, project.Name)
	} else {
		fmt.Fprintf(log, "Uninstalling the environment of %s\n", project.Name)
	}
	if err := s.deps.Executor.UninstallEnvironment(ctx, project, log); err != nil {
		return err
	}
	updates := map[string]any{"installed_version": nil, "size_bytes": 0}
	if project.InstalledVersion != nil {
		updates["last_installed_version"] = *project.InstalledVersion
	}
	if err := s.db(ctx).Model(&models.Project{}).Where("id = ?", project.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("recording uninstall: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------- helpers

func (s *Service) db(ctx context.Context) *gorm.DB { return s.deps.DB.WithContext(ctx) }

func (s *Service) project(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	var p models.Project
	err := s.db(ctx).Where("id = ?", id).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("project %s: %w", id, shared.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// checkIdle rejects a new install/uninstall while the project is being
// created or deleted, or another job is queued or running on it.
func (s *Service) checkIdle(ctx context.Context, project *models.Project) error {
	if project.Status == models.ProjectStatusPending || project.Status == models.ProjectStatusCreating || project.Status == models.ProjectStatusDeleting {
		return &shared.ConflictError{Code: "job_active", Message: fmt.Sprintf("%s is busy. Wait for its current job to finish.", project.Name)}
	}
	var count int64
	if err := s.db(ctx).Model(&models.Job{}).
		Where("project_id = ? AND status IN ?", project.ID, []models.JobStatus{models.JobStatusPending, models.JobStatusRunning}).
		Count(&count).Error; err != nil {
		return fmt.Errorf("checking active jobs: %w", err)
	}
	if count > 0 {
		return &shared.ConflictError{Code: "job_active", Message: fmt.Sprintf("Another job is running on %s. Wait for it to finish.", project.Name)}
	}
	return nil
}

// resolveInstallVersion picks the version to install: the requested one,
// else the last installed version (if still stored), else the latest.
func (s *Service) resolveInstallVersion(ctx context.Context, project *models.Project, requested *int) (int, error) {
	if requested != nil {
		if *requested < 1 {
			return 0, &shared.ValidationError{Field: "version", Message: "Version numbers start at 1."}
		}
		return *requested, nil
	}
	if project.LastInstalledVersion != nil {
		if _, err := s.local.LoadVersion(ctx, project.ID, *project.LastInstalledVersion); err == nil {
			return *project.LastInstalledVersion, nil
		}
	}
	var latest models.ProjectVersion
	err := s.db(ctx).Select("version_number").Where("project_id = ?", project.ID).Order("version_number DESC").First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, &shared.ValidationError{Field: "version", Message: fmt.Sprintf("%s has no versions on this machine yet.", project.Name)}
	}
	if err != nil {
		return 0, err
	}
	return latest.VersionNumber, nil
}

// writeAndInstall writes the version's files into dir and installs them.
func (s *Service) writeAndInstall(ctx context.Context, project *models.Project, dir string, v *models.ProjectVersion, log io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	locked := len(bytes.TrimSpace([]byte(v.LockFileContent))) > 0
	if err := writeVersionFiles(dir, []byte(v.ManifestContent), []byte(v.LockFileContent)); err != nil {
		return err
	}
	fmt.Fprintf(log, "Wrote %s and %s of version %d\n", artifact.ManifestFile, artifact.LockFile, v.VersionNumber)
	if fi, ok := s.deps.Executor.(frozenInstaller); ok && locked {
		return fi.InstallEnvironmentFrozen(ctx, project, log)
	}
	if !locked {
		fmt.Fprintf(log, "Version %d has no %s; pixi will solve it during the install\n", v.VersionNumber, artifact.LockFile)
	}
	return s.deps.Executor.InstallEnvironment(ctx, project, log)
}

// rollbackInstall puts the project back the way it was after a failed or
// cancelled install. It runs detached from ctx so a cancelled job still
// cleans up.
func (s *Service) rollbackInstall(ctx context.Context, project *models.Project, dir string, attempted int, previous *int, before fileSnapshot, envExisted bool, log io.Writer) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreTimeout)
	defer cancel()

	if previous == nil {
		if err := before.restore(dir); err != nil {
			fmt.Fprintf(log, "Couldn't restore the previous project files: %v\n", err)
		}
		if !envExisted {
			fmt.Fprintf(log, "Removing the partly installed environment\n")
			if err := s.deps.Executor.UninstallEnvironment(rctx, project, log); err != nil {
				fmt.Fprintf(log, "Couldn't remove the partly installed environment: %v\n", err)
			}
		}
		return
	}

	if *previous == attempted {
		// Reinstalling the installed version failed: its files are already
		// in place and no other version replaced the environment.
		fmt.Fprintf(log, "Version %d stays installed\n", *previous)
		return
	}
	if !envExisted {
		// The recorded version was not on disk anyway.
		_ = before.restore(dir)
		s.markNotInstalled(rctx, project, previous, log)
		return
	}
	prev, err := s.local.LoadVersion(rctx, project.ID, *previous)
	if err != nil {
		fmt.Fprintf(log, "Version %d is no longer stored; the project is not installed now: %v\n", *previous, err)
		_ = before.restore(dir)
		s.markNotInstalled(rctx, project, previous, log)
		return
	}
	fmt.Fprintf(log, "Reinstalling version %d, which was installed before\n", *previous)
	if err := s.writeAndInstall(rctx, project, dir, prev, log); err != nil {
		fmt.Fprintf(log, "Couldn't reinstall version %d: %v\n", *previous, err)
		s.markNotInstalled(rctx, project, previous, log)
		return
	}
	fmt.Fprintf(log, "Version %d is installed again\n", *previous)
}

func (s *Service) markNotInstalled(ctx context.Context, project *models.Project, previous *int, log io.Writer) {
	updates := map[string]any{"installed_version": nil, "size_bytes": s.envSize(project)}
	if previous != nil {
		updates["last_installed_version"] = *previous
	}
	if err := s.db(ctx).Model(&models.Project{}).Where("id = ?", project.ID).Updates(updates).Error; err != nil {
		fmt.Fprintf(log, "Couldn't record the install state: %v\n", err)
	}
}

// envSize is the size of the installed environment (.pixi/envs) in bytes.
func (s *Service) envSize(project *models.Project) int64 {
	envs := filepath.Join(s.deps.Executor.GetProjectPath(project), ".pixi", "envs")
	if _, err := os.Stat(envs); err != nil {
		return 0
	}
	size, err := utils.GetDirectorySize(envs)
	if err != nil {
		return 0
	}
	return size
}

// summarize converts a submitted job. Inline (CLI) jobs come back finished
// together with the job error; both are returned.
func summarize(job *models.Job, projectName string, err error) (*shared.JobSummary, error) {
	if job == nil {
		return nil, err
	}
	sum := shared.JobSummaryFrom(*job, projectName)
	return &sum, err
}

// writeVersionFiles writes pixi.toml and pixi.lock (removing a stale lock
// when the version has none) atomically enough for one writer.
func writeVersionFiles(dir string, manifest, lock []byte) error {
	if err := writeFileAtomic(filepath.Join(dir, artifact.ManifestFile), manifest); err != nil {
		return fmt.Errorf("writing %s: %w", artifact.ManifestFile, err)
	}
	lockPath := filepath.Join(dir, artifact.LockFile)
	if len(bytes.TrimSpace(lock)) == 0 {
		if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing stale %s: %w", artifact.LockFile, err)
		}
		return nil
	}
	if err := writeFileAtomic(lockPath, lock); err != nil {
		return fmt.Errorf("writing %s: %w", artifact.LockFile, err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// fileSnapshot holds pixi.toml / pixi.lock as they were before an install
// (nil = the file did not exist).
type fileSnapshot struct {
	manifest, lock []byte
}

func snapshotFiles(dir string) fileSnapshot {
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil
		}
		if b == nil {
			b = []byte{}
		}
		return b
	}
	return fileSnapshot{manifest: read(artifact.ManifestFile), lock: read(artifact.LockFile)}
}

func (f fileSnapshot) restore(dir string) error {
	put := func(name string, data []byte) error {
		path := filepath.Join(dir, name)
		if data == nil {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		return writeFileAtomic(path, data)
	}
	if err := put(artifact.ManifestFile, f.manifest); err != nil {
		return err
	}
	return put(artifact.LockFile, f.lock)
}
