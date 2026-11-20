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

// CreateVersionResult is what CreateVersion and SnapshotWorkingCopy return.
type CreateVersionResult struct {
	// Version is set when no solve was needed (unchanged copy of the base,
	// or a result identical to the latest version).
	Version *ProjectVersion `json:"version,omitempty"`
	// Job is set when a solve is running. The version is recorded when the
	// job succeeds, and the job result carries its ID. When Version is set
	// and Install was asked for, Job is the install job.
	Job *jobs.Job `json:"job,omitempty"`
	// Deduplicated is true when the result matched the latest version and
	// no new version was created.
	Deduplicated bool `json:"deduplicated"`
}

// CreateVersion makes a new version of a project from a base version.
//
//  1. The base must be a version of the project. When it is not the latest
//     version, req.AllowOlderBase must be set, or ErrOlderBase is returned.
//  2. The new pixi.toml is the Spec applied to the base's pixi.toml
//     (tables the Spec does not cover are kept), or req.Manifest as given,
//     or the base's own when neither is set.
//  3. When the pixi.toml is unchanged from the base, the version is
//     recorded at once with the base's lock and no solve. When its content
//     matches the latest version the result is Deduplicated and nothing is
//     written; otherwise this is how an older version becomes the newest
//     one again.
//  4. Otherwise a create_version job solves the lock file and records the
//     version and its tags in one transaction, or reports Deduplicated
//     when the result matches the latest version (see createVersionJob).
//
// With req.Install the recorded version is then installed into the project
// directory. Install replaces pixi.toml and pixi.lock there, so it is
// refused with ErrWorkingCopyModified when they hold changes that are not
// in any recorded version; snapshot them first.
//
// Tags are checked before anything runs: an invalid one returns
// ErrInvalidTag, and one already on another version ErrTagExists.
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
	if req.Install {
		if err := s.checkReplaceable(ctx, p.ID, p.Path); err != nil {
			return nil, err
		}
	}

	if manifest == base.Manifest {
		return s.copyVersion(ctx, p, base, req, tags)
	}

	inPlace, err := artifact.HasLocalPaths(manifest)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if inPlace {
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
		inPlace:     inPlace,
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
	var dedup bool
	err := s.versionTx(ctx, func(tx *gorm.DB) error {
		var err error
		v, dedup, err = s.recordVersion(tx, recordVersionRequest{
			ProjectID:     p.ID,
			BaseVersionID: &base.ID,
			Manifest:      base.Manifest,
			Lock:          base.Lock,
			Origin:        OriginCreated,
			Author:        s.author,
			Description:   req.Description,
			Tags:          tags,
			DedupeLatest:  true,
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("recording the version: %w", err)
	}
	res := &CreateVersionResult{Version: v, Deduplicated: dedup}
	if !req.Install {
		return res, nil
	}
	job, err := s.jobs.Submit(ctx, p.ID, jobs.TypeInstall, func(ctx context.Context, log io.Writer) error {
		return s.installStep(ctx, p.ID, p.Path, v, log)
	})
	if err != nil {
		return res, fmt.Errorf("version %d is recorded, but its install could not be started: %w", v.Number, err)
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
	// inPlace locks in the project directory instead of a temporary one,
	// for manifests with local paths (see artifact.HasLocalPaths).
	inPlace bool
}

// createVersionJob is the body of a create_version job. It solves the new
// pixi.toml starting from the base's pixi.lock, so unrelated packages stay
// pinned, and records the result in one transaction with its tags, or
// reports Deduplicated when the result matches the latest version. With
// install it then runs the install step.
//
// The solve runs in a temporary directory, so the project directory and
// its environment stay on the installed version until the new one is
// installed. A manifest with local paths, such as path dependencies or a
// relative channel, is solved in the project
// directory instead (see solveInPlace).
func (s *Service) createVersionJob(j versionJob) jobs.Func {
	return func(ctx context.Context, log io.Writer) error {
		jobs.ReportStage(ctx, StageLock)
		var solved workingCopy
		var err error
		if j.inPlace {
			solved, err = s.solveInPlace(ctx, j, log)
		} else {
			solved, err = s.solveInTemp(ctx, j, log)
		}
		if err != nil {
			return err
		}

		var v *ProjectVersion
		var dedup bool
		err = s.versionTx(ctx, func(tx *gorm.DB) error {
			var err error
			v, dedup, err = s.recordVersion(tx, recordVersionRequest{
				ProjectID:     j.projectID,
				BaseVersionID: &j.base.ID,
				Manifest:      solved.Manifest,
				Lock:          solved.Lock,
				Origin:        OriginCreated,
				Author:        s.author,
				Description:   j.description,
				Tags:          j.tags,
				DedupeLatest:  true,
			})
			return err
		})
		if err != nil {
			return fmt.Errorf("recording the version: %w", err)
		}
		if dedup {
			jobs.ReportDeduplicated(ctx, v.ID)
		} else {
			jobs.ReportVersion(ctx, v.ID)
		}
		if !j.install {
			return nil
		}
		return s.installStep(ctx, j.projectID, j.dir, v, log)
	}
}

// solveInTemp writes the new pixi.toml and the base's pixi.lock to a
// temporary directory, with the project's pixi configuration, runs pixi
// lock there and returns the result.
func (s *Service) solveInTemp(ctx context.Context, j versionJob, log io.Writer) (workingCopy, error) {
	dir, err := os.MkdirTemp("", "nebi-lock-")
	if err != nil {
		return workingCopy{}, fmt.Errorf("creating a directory to lock in: %w", err)
	}
	defer os.RemoveAll(dir)
	if err := writeVersionFiles(dir, j.manifest, j.base.Lock); err != nil {
		return workingCopy{}, err
	}
	if err := copyProjectConfig(j.dir, dir); err != nil {
		return workingCopy{}, err
	}
	if err := s.pixi.Lock(ctx, dir, log); err != nil {
		return workingCopy{}, fmt.Errorf("pixi lock: %w", err)
	}
	return readSolved(dir)
}

// solveInPlace locks a manifest with local paths in the project
// directory, where its relative paths resolve. The working copy must hold
// the files of a recorded version, so nothing unsaved is overwritten; it
// is checked again here because it may have changed while the job was
// pending. The new pixi.toml and the base's pixi.lock are written in place,
// pixi lock runs, and the original files are put back whatever happens,
// so the project directory stays on its version until install.
func (s *Service) solveInPlace(ctx context.Context, j versionJob, log io.Writer) (solved workingCopy, err error) {
	if err := s.checkLockableInPlace(ctx, j.projectID, j.dir); err != nil {
		return workingCopy{}, err
	}
	orig, err := readWorkingCopy(j.dir)
	if err != nil {
		return workingCopy{}, err
	}
	defer func() {
		if rerr := restoreWorkingCopy(j.dir, orig); rerr != nil {
			err = errors.Join(err, rerr)
		}
	}()
	if err := writeVersionFiles(j.dir, j.manifest, j.base.Lock); err != nil {
		return workingCopy{}, err
	}
	if err := s.pixi.Lock(ctx, j.dir, log); err != nil {
		return workingCopy{}, fmt.Errorf("pixi lock: %w", err)
	}
	return readSolved(j.dir)
}

// projectConfigFile is pixi's per-project configuration (mirrors, channel
// and PyPI settings), relative to the project directory.
var projectConfigFile = filepath.Join(".pixi", "config.toml")

// copyProjectConfig copies the project's pixi configuration, when it has
// one, into dir, so a solve there uses the same settings as one in the
// project directory.
func copyProjectConfig(projectDir, dir string) error {
	src := filepath.Join(projectDir, projectConfigFile)
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", src, err)
	}
	dst := filepath.Join(dir, projectConfigFile)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("copying %s: %w", projectConfigFile, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("copying %s: %w", projectConfigFile, err)
	}
	return nil
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
		return fmt.Errorf("%w; a pixi.toml with local paths is locked in the project directory", err)
	}
	recorded, err := s.isRecorded(ctx, projectID, wc)
	if err != nil {
		return err
	}
	if !recorded {
		return fmt.Errorf("%w: a pixi.toml with local paths is locked in the project directory, which has changes that are in no version; snapshot or discard them first", ErrWorkingCopyModified)
	}
	return nil
}

// checkReplaceable allows replacing pixi.toml and pixi.lock in dir: the
// directory must exist, and the files must both be missing or hold a
// recorded version of the project (see isRecorded), so no change that
// exists only on disk is lost. A pixi.lock without a pixi.toml is refused:
// it may hold a lock that is in no version.
func (s *Service) checkReplaceable(ctx context.Context, projectID uuid.UUID, dir string) error {
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
		return fmt.Errorf("%w: %s has pixi.toml or pixi.lock changes that are in no version; snapshot them first", ErrWorkingCopyModified, dir)
	}
	return nil
}

// isRecorded reports whether wc holds nothing that is in no version of the
// project: its content matches a version by content hash or, when there is
// no pixi.lock, its pixi.toml is a version's pixi.toml, since a missing lock
// loses nothing.
func (s *Service) isRecorded(ctx context.Context, projectID uuid.UUID, wc workingCopy) (bool, error) {
	q := s.db.WithContext(ctx).Model(&ProjectVersion{}).Where("project_id = ?", projectID)
	if wc.HasLock {
		q = q.Where("content_hash = ?", wc.hash())
	} else {
		q = q.Where("manifest = ?", wc.Manifest)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, fmt.Errorf("checking the working copy: %w", err)
	}
	return n > 0, nil
}

// installStep installs version v into the project directory dir: the step
// the create job, a create_version job and an install job share.
//
// It refuses when pixi.toml or pixi.lock in dir hold changes that are in
// no recorded version. Then it reports StageInstall, clears
// InstalledVersionID before touching any file, so a crash midway does not
// leave the old version reported as installed, writes the version's
// pixi.toml and pixi.lock with atomic renames, runs pixi install and sets
// InstalledVersionID to v.
func (s *Service) installStep(ctx context.Context, projectID uuid.UUID, dir string, v *ProjectVersion, log io.Writer) error {
	if err := s.checkReplaceable(ctx, projectID, dir); err != nil {
		return err
	}
	jobs.ReportStage(ctx, StageInstall)
	if err := s.db.WithContext(ctx).Model(&Project{}).Where("id = ?", projectID).
		Update("installed_version_id", nil).Error; err != nil {
		return fmt.Errorf("clearing the installed version: %w", err)
	}
	if err := writeVersionFiles(dir, v.Manifest, v.Lock); err != nil {
		return err
	}
	if err := s.pixi.Install(ctx, dir, log); err != nil {
		return fmt.Errorf("pixi install: %w", err)
	}
	// The environment is installed: record it even if the job is
	// cancelled now, or a complete install would look failed.
	if err := s.db.WithContext(context.WithoutCancel(ctx)).Model(&Project{}).Where("id = ?", projectID).
		Update("installed_version_id", v.ID).Error; err != nil {
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
// renames it over path, so readers see the old file or the new one.
func writeFileAtomic(path, content string) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// SnapshotWorkingCopy records pixi.toml and pixi.lock as they are in the
// project directory, for users who edit pixi.toml by hand and run pixi
// themselves.
//
// It returns ErrWorkingCopyMissing when pixi.toml is gone, and ErrLockStale
// when pixi.lock is missing or does not satisfy pixi.toml
// (PixiRunner.CheckLock, which changes neither file): run pixi lock or
// pixi install first. It refuses with a *BusyError while a job for the
// project is pending or running, since jobs write these files.
//
// The version's origin is snapshot and its base is the installed version.
// It is deduplicated against the latest version. InstalledVersionID is not
// changed: the environment on disk may not match until the version is
// installed.
func (s *Service) SnapshotWorkingCopy(ctx context.Context, projectID uuid.UUID, description string, tags []string) (*CreateVersionResult, error) {
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
	current, err := s.pixi.CheckLock(ctx, p.Path, &out)
	if err != nil {
		return nil, fmt.Errorf("checking %s: %w%s", artifact.LockFile, err, pixiOutput(&out))
	}
	if !current {
		return nil, fmt.Errorf("%w: %s does not match %s; run pixi lock or pixi install first%s", ErrLockStale, artifact.LockFile, artifact.ManifestFile, pixiOutput(&out))
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
	var dedup bool
	err = s.versionTx(ctx, func(tx *gorm.DB) error {
		// The installed version as of the write, not of the first read.
		current, err := projectIn(tx, p.ID)
		if err != nil {
			return err
		}
		v, dedup, err = s.recordVersion(tx, recordVersionRequest{
			ProjectID:     p.ID,
			BaseVersionID: current.InstalledVersionID,
			Manifest:      wc.Manifest,
			Lock:          wc.Lock,
			Origin:        OriginSnapshot,
			Author:        s.author,
			Description:   description,
			Tags:          tags,
			DedupeLatest:  true,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return &CreateVersionResult{Version: v, Deduplicated: dedup}, nil
}

// checkIdle returns a *BusyError when the project has a pending or running
// job.
func (s *Service) checkIdle(ctx context.Context, projectID uuid.UUID) error {
	list, err := s.jobs.ProjectJobs(ctx, projectID)
	if err != nil {
		return fmt.Errorf("reading jobs of project %s: %w", projectID, err)
	}
	var active []uuid.UUID
	for _, j := range list {
		if j.Active() {
			active = append(active, j.ID)
		}
	}
	if len(active) > 0 {
		return &BusyError{ProjectID: projectID, JobIDs: active}
	}
	return nil
}

// pixiOutput formats what pixi printed for an error message.
func pixiOutput(out *bytes.Buffer) string {
	text := strings.TrimSpace(out.String())
	if text == "" {
		return ""
	}
	return "\n" + text
}
