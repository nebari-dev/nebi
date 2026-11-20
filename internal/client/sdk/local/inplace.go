package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"gorm.io/gorm"
)

// pendingRestore records an in-place solve (see solveInPlace) that replaced
// pixi.toml and pixi.lock in a project directory, so the files can be put
// back if the process stops before the solve does. RecoverInPlaceSolves
// finishes it.
type pendingRestore struct {
	// ProjectID is the project; a project runs one job at a time, so it has
	// at most one in-place solve.
	ProjectID uuid.UUID `gorm:"type:text;primaryKey"`
	// VersionID is the recorded version whose files were in the directory.
	VersionID uuid.UUID `gorm:"type:text;not null"`
	// HadLock is false when the directory had no pixi.lock.
	HadLock bool `gorm:"not null"`
	// Written is the digest of the pixi.toml the solve wrote. The files are
	// put back only while pixi.toml still has it, so a later edit is kept.
	Written   string `gorm:"not null"`
	CreatedAt time.Time
}

func (pendingRestore) TableName() string { return "client_pending_restores" }

func digest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// beginInPlace records a pendingRestore for the working copy, which must
// be a recorded version, and then writes the new pixi.toml and the base's
// pixi.lock into the project directory. It returns the working copy it
// replaced and the digest of the pixi.toml it wrote.
func (s *Service) beginInPlace(ctx context.Context, j versionJob) (workingCopy, string, error) {
	orig, err := readWorkingCopy(j.dir)
	if err != nil {
		return workingCopy{}, "", err
	}
	versionID, err := s.recordedVersionID(ctx, j.projectID, orig)
	if err != nil {
		return workingCopy{}, "", err
	}
	if versionID == uuid.Nil {
		return workingCopy{}, "", fmt.Errorf("%w: the project directory changed before it could be locked in", ErrWorkingCopyModified)
	}
	written := digest(j.manifest)
	r := pendingRestore{ProjectID: j.projectID, VersionID: versionID, HadLock: orig.HasLock, Written: written}
	if err := s.db.WithContext(ctx).Create(&r).Error; err != nil {
		return workingCopy{}, "", fmt.Errorf("recording the in-place solve: %w", err)
	}
	if err := writeVersionFiles(j.dir, j.manifest, j.base.Lock); err != nil {
		return workingCopy{}, "", errors.Join(err, s.finishInPlace(ctx, j.projectID, j.dir, written, orig))
	}
	return orig, written, nil
}

// finishInPlace puts orig back in dir unless pixi.toml has changed since
// the solve wrote it (digest written), and drops the pendingRestore. When
// the files cannot be written the record is kept for RecoverInPlaceSolves.
func (s *Service) finishInPlace(ctx context.Context, projectID uuid.UUID, dir, written string, orig workingCopy) error {
	outcome, err := putBack(dir, written, orig)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(context.WithoutCancel(ctx)).Delete(&pendingRestore{}, "project_id = ?", projectID).Error; err != nil {
		return fmt.Errorf("dropping the in-place solve record: %w", err)
	}
	if outcome == putBackEdited {
		return fmt.Errorf("%w: %s was changed during the lock, so %s and %s were left as they are", ErrWorkingCopyModified, artifact.ManifestFile, artifact.ManifestFile, artifact.LockFile)
	}
	return nil
}

type putBackOutcome int

const (
	putBackDone      putBackOutcome = iota // the files were put back
	putBackUntouched                       // pixi.toml is still the original one
	putBackEdited                          // pixi.toml was changed or removed by someone else
)

// putBack writes orig into dir when pixi.toml still has digest written,
// the one an in-place solve wrote. It changes nothing when pixi.toml is
// the original one already or was changed by someone else.
func putBack(dir, written string, orig workingCopy) (putBackOutcome, error) {
	current, err := os.ReadFile(filepath.Join(dir, artifact.ManifestFile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return putBackEdited, nil
	case err != nil:
		return 0, fmt.Errorf("reading %s: %w", artifact.ManifestFile, err)
	}
	if digest(string(current)) != written {
		if string(current) == orig.Manifest {
			return putBackUntouched, nil
		}
		return putBackEdited, nil
	}
	if err := restoreWorkingCopy(dir, orig); err != nil {
		return 0, err
	}
	return putBackDone, nil
}

// RecoverInPlaceSolves puts back the files of in-place solves that an
// earlier run stopped in the middle of (see solveInPlace). Call it once at
// startup, like RecoverDeletes. A project whose pixi.toml was changed since
// is left as it is. Projects with an active job are skipped: the job puts
// its files back itself.
func (s *Service) RecoverInPlaceSolves(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var pending []pendingRestore
	if err := s.db.WithContext(ctx).Order("created_at").Find(&pending).Error; err != nil {
		return fmt.Errorf("reading in-place solve records: %w", err)
	}
	var errs []error
	for _, r := range pending {
		if err := s.recoverInPlace(ctx, r); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) recoverInPlace(ctx context.Context, r pendingRestore) error {
	list, err := s.jobs.ProjectJobs(ctx, r.ProjectID)
	if err != nil {
		return fmt.Errorf("reading jobs of project %s: %w", r.ProjectID, err)
	}
	for _, j := range list {
		if j.Active() {
			return nil
		}
	}
	drop := func() error {
		if err := s.db.WithContext(ctx).Delete(&pendingRestore{}, "project_id = ?", r.ProjectID).Error; err != nil {
			return fmt.Errorf("dropping the in-place solve record of project %s: %w", r.ProjectID, err)
		}
		return nil
	}
	p, err := s.loadProject(ctx, r.ProjectID)
	if errors.Is(err, ErrNotFound) {
		return drop()
	}
	if err != nil {
		return err
	}
	v, err := versionByID(s.db.WithContext(ctx), p.ID, r.VersionID)
	if errors.Is(err, ErrNotFound) {
		return drop()
	}
	if err != nil {
		return err
	}
	orig := workingCopy{Manifest: v.Manifest}
	if r.HadLock {
		orig.Lock, orig.HasLock = v.Lock, true
	}
	if _, err := putBack(p.Path, r.Written, orig); err != nil {
		return fmt.Errorf("putting back the files of project %s: %w", p.Name, err)
	}
	return drop()
}

// deletePendingRestores drops the project's in-place solve record inside
// tx, the transaction that deletes the project row.
func deletePendingRestores(tx *gorm.DB, projectID uuid.UUID) error {
	if err := tx.Where("project_id = ?", projectID).Delete(&pendingRestore{}).Error; err != nil {
		return fmt.Errorf("deleting in-place solve records: %w", err)
	}
	return nil
}
