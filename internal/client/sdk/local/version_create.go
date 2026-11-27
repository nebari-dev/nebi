package local

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"gorm.io/gorm"
)

// CreateVersionRequest describes a new version made from a base version.
type CreateVersionRequest struct {
	ProjectID     uuid.UUID
	BaseVersionID uuid.UUID // required

	// AllowOlderBase must be true when the base is not the latest version.
	// Without it, CreateVersion returns ErrOlderBase, so every caller has to
	// confirm.
	AllowOlderBase bool

	// At most one of Spec (structured input) or Manifest (raw pixi.toml) is set.
	// Neither means an unchanged copy of the base: no solve, the base's
	// lock is reused, and the copy becomes the latest version.
	Spec     *artifact.Spec
	Manifest string

	Description string
	Tags        []string
	// Install installs the new version once it is recorded, with the
	// install step the create job also runs.
	Install bool
}

// CreateVersionResult is what CreateVersion and CreateResolvedVersion
// return. It has one of three shapes:
//
//   - Version set, Job nil: the version is recorded (no solve was needed).
//     The client API answers 201.
//   - Job set, Version nil: a create_version job is solving the lock file
//     and records the version when it succeeds; the job carries its ID.
//     The client API answers 202.
//   - Version and Job set: CreateVersion with Install recorded the version
//     at once (as in the first shape) and Job is the install job installing
//     it. The client API answers 202, with the version in the body.
//
// InstallError is set instead of Job when the version was recorded but its
// install could not be started; CreateVersion then returns no error,
// because the version exists.
type CreateVersionResult struct {
	Version *ProjectVersion `json:"version,omitempty"`
	Job     *jobs.Job       `json:"job,omitempty"`
	// InstallError says why the install of a recorded Version could not be
	// started.
	InstallError string `json:"install_error,omitempty"`
}

// CreateVersion makes a new version of a project from a base version.
//
//  1. The base must be a version of the project. When it is not the latest
//     version, req.AllowOlderBase must be set, or ErrOlderBase is returned.
//  2. The new pixi.toml is the Spec applied to the base's pixi.toml
//     (tables the Spec does not cover are kept), or req.Manifest as given,
//     or the base's own when neither is set.
//  3. When the pixi.toml is unchanged from the base, the version is
//     recorded at once with the base's lock and no solve. This is how an
//     older version becomes the newest one again.
//  4. Otherwise a create_version job solves the lock file in the project
//     directory and records the version and its tags in one transaction
//     (see createVersionJob). The solve writes pixi.toml and pixi.lock
//     there, so it is refused with ErrWorkingCopyModified when they hold
//     changes that are in no recorded version.
//
// With req.Install the recorded version is then installed into the project
// directory. Install replaces pixi.toml and pixi.lock there, so it is
// refused with ErrWorkingCopyModified when they hold changes that are not
// in any recorded version; record them with CreateResolvedVersion first.
//
// The project directory is checked up front only when the project has no
// pending or running job; otherwise the submitted job checks it when it
// starts and fails with ErrWorkingCopyModified.
//
// Tags are checked before anything runs: an invalid one returns
// ErrInvalidTag, and one already on another version ErrTagExists.
//
// Besides the errors in #661's table, CreateVersion returns
// ErrInvalidRequest when BaseVersionID is not set, ErrWorkingCopyModified
// (#682) as above, and ErrWorkingCopyMissing when Install is asked for and
// the project directory is gone.
func (s *Service) CreateVersion(ctx context.Context, req CreateVersionRequest) (*CreateVersionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.loadProject(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}
	tags, err := checkTags(req.Tags)
	if err != nil {
		return nil, err
	}
	if req.BaseVersionID == uuid.Nil {
		return nil, fmt.Errorf("%w: a base version is required", ErrInvalidRequest)
	}
	db := s.db.WithContext(ctx)
	base, err := versionByID(db, p.ID, req.BaseVersionID)
	if err != nil {
		return nil, err
	}
	latest, err := latestVersion(db, p.ID, summaryColumns...)
	if err != nil {
		return nil, err
	}
	if latest != nil && base.Number != latest.Number && !req.AllowOlderBase {
		return nil, fmt.Errorf("%w: version %d is not the latest version (%d); confirm to create from it", ErrOlderBase, base.Number, latest.Number)
	}
	manifest, err := versionManifest(base.Manifest, req)
	if err != nil {
		return nil, err
	}
	if err := tagsFree(db, p.ID, uuid.Nil, tags); err != nil {
		return nil, err
	}
	// The project directory is checked here only when no job is pending or
	// running. A running create_version job holds its own pixi.toml there
	// while it solves, which is in no version yet and would be refused as
	// an unsaved edit. Jobs of a project run one at a time, and the job
	// submitted here checks the directory again when it starts, after the
	// earlier job has put its files back.
	active, err := s.activeJobIDs(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	checkDir := len(active) == 0
	if req.Install && checkDir {
		if err := s.checkReplaceable(ctx, p.ID, p.Path); err != nil {
			return nil, err
		}
	}

	if manifest == base.Manifest {
		return s.copyVersion(ctx, p, base, req, tags)
	}

	if checkDir {
		if err := s.checkLockableInPlace(ctx, p.ID, p.Path); err != nil {
			return nil, err
		}
	}
	job, err := s.jobs.Submit(ctx, p.ID, jobs.TypeCreateVersion, s.createVersionJob(versionJob{
		projectID:   p.ID,
		dir:         p.Path,
		base:        base,
		manifest:    manifest,
		description: req.Description,
		tags:        tags,
		install:     req.Install,
	}))
	if err != nil {
		return nil, fmt.Errorf("submitting create_version job: %w", err)
	}
	return &CreateVersionResult{Job: job}, nil
}

// copyVersion records an unchanged copy of base, with base's lock and no
// solve, and submits an install job for it when asked.
func (s *Service) copyVersion(ctx context.Context, p *Project, base *ProjectVersion, req CreateVersionRequest, tags []string) (*CreateVersionResult, error) {
	var v *ProjectVersion
	err := s.versionTx(ctx, func(tx *gorm.DB) error {
		var err error
		v, err = s.recordVersion(tx, recordVersionRequest{
			ProjectID:                p.ID,
			BaseVersionID:            &base.ID,
			EnvironmentSpecification: base.EnvironmentSpecification,
			Origin:                   OriginCreated,
			Author:                   s.author,
			Description:              req.Description,
			Tags:                     tags,
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("recording the version: %w", err)
	}
	res := &CreateVersionResult{Version: v}
	if !req.Install {
		return res, nil
	}
	job, err := s.jobs.Submit(ctx, p.ID, jobs.TypeInstall, func(ctx context.Context, log io.Writer) error {
		return s.installStep(ctx, p.ID, p.Path, v, false, log)
	})
	if err != nil {
		// The version exists, so this is not a failed create.
		res.InstallError = fmt.Sprintf("version %d is recorded, but its install could not be started: %v", v.Number, err)
		return res, nil
	}
	res.Job = job
	return res, nil
}

// versionManifest returns the pixi.toml of the new version: the Spec
// applied to the base's, the raw Manifest as given, or the base's own.
func versionManifest(base string, req CreateVersionRequest) (string, error) {
	hasSpec := req.Spec != nil
	hasManifest := strings.TrimSpace(req.Manifest) != ""
	switch {
	case hasSpec && hasManifest:
		return "", fmt.Errorf("%w: set at most one of Spec or Manifest", ErrInvalidManifest)
	case hasSpec:
		if !hasPlatform(req.Spec.Platforms) {
			return "", ErrNoPlatform
		}
		out, err := artifact.ApplySpec(base, *req.Spec)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidManifest, err)
		}
		return out, nil
	case hasManifest:
		parsed, err := artifact.ParseManifest(req.Manifest)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidManifest, err)
		}
		if !hasPlatform(parsed.Platforms) {
			return "", ErrNoPlatform
		}
		if err := parsed.Validate(); err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidManifest, err)
		}
		return req.Manifest, nil
	default:
		return base, nil
	}
}

// versionJob is what a create_version job needs.
type versionJob struct {
	projectID   uuid.UUID
	dir         string // the project directory
	base        *ProjectVersion
	manifest    string
	description string
	tags        []string
	install     bool
}

// createVersionJob is the body of a create_version job. It solves the new
// pixi.toml starting from the base's pixi.lock, so unrelated packages stay
// pinned, and records the result in one transaction with its tags. With
// install it then runs the install step.
//
// The solve runs in the project directory, where relative paths in the
// manifest resolve, and puts the original files back afterwards, so the
// project directory stays on its version until the new one is installed
// (see solveInPlace).
func (s *Service) createVersionJob(j versionJob) jobs.Func {
	return func(ctx context.Context, log io.Writer) error {
		jobs.ReportStage(ctx, StageLock)
		solved, err := s.solveInPlace(ctx, j, log)
		if err != nil {
			return err
		}

		var v *ProjectVersion
		err = s.versionTx(ctx, func(tx *gorm.DB) error {
			var err error
			v, err = s.recordVersion(tx, recordVersionRequest{
				ProjectID:                j.projectID,
				BaseVersionID:            &j.base.ID,
				EnvironmentSpecification: solved.EnvironmentSpecification,
				Origin:                   OriginCreated,
				Author:                   s.author,
				Description:              j.description,
				Tags:                     j.tags,
			})
			return err
		})
		if err != nil {
			return fmt.Errorf("recording the version: %w", err)
		}
		jobs.ReportVersion(ctx, v.ID)
		if !j.install {
			return nil
		}
		return s.installStep(ctx, j.projectID, j.dir, v, false, log)
	}
}

// solveInPlace locks the new pixi.toml in the project directory, where its
// relative paths resolve. The working copy must hold
// the files of a recorded version, so nothing unsaved is overwritten; it
// is checked again here because it may have changed while the job was
// pending. The new pixi.toml and the base's pixi.lock are written in place
// (see beginInPlace), pixi lock runs, and the original files are put back
// however the solve ends, so the project directory stays on its version
// until install. If pixi.toml is changed while the lock runs, the solve
// fails and the files are left as they are, so the change is kept.
func (s *Service) solveInPlace(ctx context.Context, j versionJob, log io.Writer) (solved workingCopy, err error) {
	if err := s.checkLockableInPlace(ctx, j.projectID, j.dir); err != nil {
		return workingCopy{}, err
	}
	orig, written, err := s.beginInPlace(ctx, j)
	if err != nil {
		return workingCopy{}, err
	}
	defer func() {
		if ferr := s.finishInPlace(ctx, j.projectID, j.dir, written, orig); ferr != nil {
			err = errors.Join(err, ferr)
		}
	}()
	if err := s.pixi.Lock(ctx, j.dir, log); err != nil {
		return workingCopy{}, fmt.Errorf("pixi lock: %w", err)
	}
	solved, err = readSolved(j.dir)
	if err != nil {
		return workingCopy{}, err
	}
	if solved.Manifest != j.manifest {
		// finishInPlace reports that the files were left as they are.
		return workingCopy{}, fmt.Errorf("%s changed while it was being locked", artifact.ManifestFile)
	}
	return solved, nil
}

// readSolved reads pixi.toml and pixi.lock after a solve. pixi lock must
// have written the lock file.
func readSolved(dir string) (workingCopy, error) {
	wc, err := readWorkingCopy(dir)
	if err != nil {
		return workingCopy{}, err
	}
	if !wc.HasLock {
		return workingCopy{}, fmt.Errorf("pixi lock wrote no %s", artifact.LockFile)
	}
	return wc, nil
}

// restoreWorkingCopy puts back pixi.toml and pixi.lock as they were read.
func restoreWorkingCopy(dir string, wc workingCopy) error {
	if err := writeFileAtomic(filepath.Join(dir, artifact.ManifestFile), wc.Manifest); err != nil {
		return fmt.Errorf("restoring %s: %w", artifact.ManifestFile, err)
	}
	lockPath := filepath.Join(dir, artifact.LockFile)
	if wc.HasLock {
		if err := writeFileAtomic(lockPath, wc.Lock); err != nil {
			return fmt.Errorf("restoring %s: %w", artifact.LockFile, err)
		}
		return nil
	}
	if err := os.Remove(lockPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("restoring %s: %w", artifact.LockFile, err)
	}
	return nil
}

// checkLockableInPlace allows locking in the project directory only when
// pixi.toml is there and the working copy is the content of a recorded
// version of the project, so the files can be put back as they were and
// no change that exists only on disk is at risk.
func (s *Service) checkLockableInPlace(ctx context.Context, projectID uuid.UUID, dir string) error {
	wc, err := readWorkingCopy(dir)
	if err != nil {
		return fmt.Errorf("%w; a new version is locked in the project directory", err)
	}
	recorded, err := s.isRecorded(ctx, projectID, wc)
	if err != nil {
		return err
	}
	if !recorded {
		return fmt.Errorf("%w: a new version is locked in the project directory, which has changes that are in no version; record them as a resolved version or discard them first", ErrWorkingCopyModified)
	}
	return nil
}

// checkReplaceable allows replacing pixi.toml and pixi.lock in dir: the
// directory must exist, and the files must both be missing or hold a
// recorded version of the project (see isRecorded), so no change that
// exists only on disk is lost. A pixi.lock without a pixi.toml is refused:
// it may hold a lock that is in no version.
func (s *Service) checkReplaceable(ctx context.Context, projectID uuid.UUID, dir string) error {
	if err := checkProjectDir(dir); err != nil {
		return err
	}
	wc, err := readWorkingCopy(dir)
	if errors.Is(err, ErrWorkingCopyMissing) {
		lockPath := filepath.Join(dir, artifact.LockFile)
		if _, err := os.Lstat(lockPath); err == nil {
			return fmt.Errorf("%w: %s has a %s but no %s; remove it or restore %s first", ErrWorkingCopyModified, dir, artifact.LockFile, artifact.ManifestFile, artifact.ManifestFile)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("checking %s: %w", lockPath, err)
		}
		return nil
	}
	if err != nil {
		return err
	}
	recorded, err := s.isRecorded(ctx, projectID, wc)
	if err != nil {
		return err
	}
	if !recorded {
		return fmt.Errorf("%w: %s has pixi.toml or pixi.lock changes that are in no version; record them as a resolved version first", ErrWorkingCopyModified, dir)
	}
	return nil
}

// checkInstallable is the check an install makes before it writes into
// dir: checkReplaceable, or with force only that dir exists, since the
// changes there are to be overwritten.
func (s *Service) checkInstallable(ctx context.Context, projectID uuid.UUID, dir string, force bool) error {
	if force {
		return checkProjectDir(dir)
	}
	return s.checkReplaceable(ctx, projectID, dir)
}

// checkProjectDir returns ErrWorkingCopyMissing when the project directory
// dir is gone or is not a directory.
func checkProjectDir(dir string) error {
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: project directory %s is gone", ErrWorkingCopyMissing, dir)
	}
	if err != nil {
		return fmt.Errorf("checking %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a directory", ErrWorkingCopyMissing, dir)
	}
	return nil
}

// isRecorded reports whether wc holds nothing that is in no version of the
// project: its content matches a version by content hash or, when there is
// no pixi.lock, its pixi.toml is a version's pixi.toml, since a missing lock
// loses nothing.
func (s *Service) isRecorded(ctx context.Context, projectID uuid.UUID, wc workingCopy) (bool, error) {
	id, err := s.recordedVersionID(ctx, projectID, wc)
	return id != uuid.Nil, err
}

// recordedVersionID returns the ID of the newest version of the project
// that wc matches as isRecorded does, or uuid.Nil when there is none.
func (s *Service) recordedVersionID(ctx context.Context, projectID uuid.UUID, wc workingCopy) (uuid.UUID, error) {
	q := s.db.WithContext(ctx).Model(&ProjectVersion{}).Where("project_id = ?", projectID)
	if wc.HasLock {
		q = q.Where("content_hash = ?", wc.Hash())
	} else {
		q = q.Where("manifest = ?", wc.Manifest)
	}
	var ids []uuid.UUID
	if err := q.Order("number DESC").Limit(1).Pluck("id", &ids).Error; err != nil {
		return uuid.Nil, fmt.Errorf("checking the working copy: %w", err)
	}
	if len(ids) == 0 {
		return uuid.Nil, nil
	}
	return ids[0], nil
}

// installStep installs version v into the project directory dir: the step
// the create job, a create_version job and an install job share.
//
// It refuses when pixi.toml or pixi.lock in dir hold changes that are in
// no recorded version, unless force is set, in which case they are
// overwritten; it refuses with ErrWorkingCopyMissing either way when the
// directory is gone. The check is made here, in the job, even when the
// caller made it before submitting, because the files may have changed
// while the job was pending. Then, before touching any file, it clears
// InstalledVersionID and sets InstallingVersionID to v in one statement,
// so a crash midway neither leaves the old version reported as installed
// nor loses the failure, and only after that reports StageInstall: a job
// that fails that statement changed nothing, so it counts as a refusal
// and the state is the same before and after a restart. It writes the version's pixi.toml and pixi.lock with atomic renames and
// runs pixi install, which never changes pixi.lock. On success it sets
// InstalledVersionID and LastInstalledVersionID to v and clears
// InstallingVersionID in one statement. A failed, cancelled or interrupted
// install leaves InstalledVersionID nil, InstallingVersionID set,
// LastInstalledVersionID as it was and v's files in place.
func (s *Service) installStep(ctx context.Context, projectID uuid.UUID, dir string, v *ProjectVersion, force bool, log io.Writer) error {
	if err := s.checkInstallable(ctx, projectID, dir, force); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Model(&Project{}).Where("id = ?", projectID).
		Updates(map[string]any{
			"installed_version_id":  nil,
			"installing_version_id": v.ID,
		}).Error; err != nil {
		return fmt.Errorf("recording the start of the install: %w", err)
	}
	jobs.ReportStage(ctx, StageInstall)
	if err := writeVersionFiles(dir, v.Manifest, v.Lock); err != nil {
		return err
	}
	if err := s.pixi.Install(ctx, dir, log); err != nil {
		return fmt.Errorf("pixi install: %w", err)
	}
	// The environment is installed: record it even if the job is
	// cancelled now, or a complete install would look failed. One UPDATE
	// sets all three columns, so they cannot disagree.
	if err := s.db.WithContext(context.WithoutCancel(ctx)).Model(&Project{}).Where("id = ?", projectID).
		Updates(map[string]any{
			"installed_version_id":      v.ID,
			"last_installed_version_id": v.ID,
			"installing_version_id":     nil,
		}).Error; err != nil {
		return fmt.Errorf("marking version %d installed: %w", v.Number, err)
	}
	return nil
}

// writeVersionFiles writes pixi.toml and, when lock is not empty, pixi.lock
// into dir, each with an atomic rename. An empty lock removes pixi.lock.
func writeVersionFiles(dir, manifest, lock string) error {
	if err := writeFileAtomic(filepath.Join(dir, artifact.ManifestFile), manifest); err != nil {
		return fmt.Errorf("writing %s: %w", artifact.ManifestFile, err)
	}
	lockPath := filepath.Join(dir, artifact.LockFile)
	if lock == "" {
		if err := os.Remove(lockPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("removing %s: %w", artifact.LockFile, err)
		}
		return nil
	}
	if err := writeFileAtomic(lockPath, lock); err != nil {
		return fmt.Errorf("writing %s: %w", artifact.LockFile, err)
	}
	return nil
}

// writeFileAtomic writes content to a temporary file next to path and
// renames it over path, so readers see the old file or the new one. When
// path is a symlink the file it points at is replaced and the link is
// kept, and an existing file keeps its permissions; a new one gets 0644.
func writeFileAtomic(path, content string) error {
	target, err := filepath.EvalSymlinks(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		target = path
	case err != nil:
		return err
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".tmp-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// CreateResolvedVersion makes a new version of a project from a pixi.toml
// and pixi.lock that were already solved, for users who edit pixi.toml by
// hand and run pixi themselves. It is CreateVersion without the solve: it
// takes both files, as they are in the project directory, instead of
// solving the lock from the pixi.toml.
//
// It returns ErrWorkingCopyMissing when pixi.toml is gone, and ErrLockStale
// when pixi.lock is missing or does not satisfy pixi.toml
// (PixiRunner.CheckLock, which changes neither file, reports a
// StaleLockError): run pixi lock or pixi install first. It refuses with a
// *BusyError while a job for the project is pending or running, since jobs
// write these files, and holds off new jobs for the project until the
// version is recorded.
//
// The version's origin is resolved and its base is the installed version.
// InstalledVersionID is not changed: the environment on disk may not match
// until the version is installed.
func (s *Service) CreateResolvedVersion(ctx context.Context, projectID uuid.UUID, description string, tags []string) (*CreateVersionResult, error) {
	// Held until the version is recorded, so no job that writes these files
	// can be submitted between the idle check and the write.
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.loadProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	tags, err = checkTags(tags)
	if err != nil {
		return nil, err
	}
	if err := s.checkIdle(ctx, p.ID); err != nil {
		return nil, err
	}
	wc, err := readWorkingCopy(p.Path)
	if err != nil {
		return nil, err
	}
	if !wc.HasLock {
		return nil, fmt.Errorf("%w: there is no %s in %s; run pixi lock or pixi install first", ErrLockStale, artifact.LockFile, p.Path)
	}
	if _, err := artifact.ParseManifest(wc.Manifest); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if err := tagsFree(s.db.WithContext(ctx), p.ID, uuid.Nil, tags); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	if err := s.pixi.CheckLock(ctx, p.Path, &out); err != nil {
		if isStaleLock(err) {
			return nil, fmt.Errorf("%w: %s does not match %s; run pixi lock or pixi install first: %v%s", ErrLockStale, artifact.LockFile, artifact.ManifestFile, err, pixiOutput(&out))
		}
		return nil, fmt.Errorf("checking %s: %w%s", artifact.LockFile, err, pixiOutput(&out))
	}
	// The check ran on the files on disk; make sure they are still the
	// ones that were read.
	again, err := readWorkingCopy(p.Path)
	if err != nil {
		return nil, err
	}
	if again != wc {
		return nil, fmt.Errorf("%s or %s changed while being checked; try again", artifact.ManifestFile, artifact.LockFile)
	}

	var v *ProjectVersion
	err = s.versionTx(ctx, func(tx *gorm.DB) error {
		// The installed version as of the write, not of the first read.
		current, err := projectIn(tx, p.ID)
		if err != nil {
			return err
		}
		v, err = s.recordVersion(tx, recordVersionRequest{
			ProjectID:                p.ID,
			BaseVersionID:            current.InstalledVersionID,
			EnvironmentSpecification: wc.EnvironmentSpecification,
			Origin:                   OriginResolved,
			Author:                   s.author,
			Description:              description,
			Tags:                     tags,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return &CreateVersionResult{Version: v}, nil
}

// checkIdle returns a *BusyError when the project has a pending or running
// job.
func (s *Service) checkIdle(ctx context.Context, projectID uuid.UUID) error {
	active, err := s.activeJobIDs(ctx, projectID)
	if err != nil {
		return err
	}
	if len(active) > 0 {
		return &BusyError{ProjectID: projectID, JobIDs: active}
	}
	return nil
}

// activeJobIDs returns the IDs of the project's pending and running jobs.
func (s *Service) activeJobIDs(ctx context.Context, projectID uuid.UUID) ([]uuid.UUID, error) {
	list, err := s.jobs.ProjectJobs(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("reading jobs of project %s: %w", projectID, err)
	}
	var active []uuid.UUID
	for _, j := range list {
		if j.Active() {
			active = append(active, j.ID)
		}
	}
	return active, nil
}

// pixiOutput formats what pixi printed for an error message.
func pixiOutput(out *bytes.Buffer) string {
	text := strings.TrimSpace(out.String())
	if text == "" {
		return ""
	}
	return "\n" + text
}
