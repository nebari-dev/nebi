package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"gorm.io/gorm"
)

// CreateVersion creates a new project version snapshot. If the most recent
// version for the project has the same content hash, it is returned and a
// new record is NOT created (deduplication). The returned bool indicates
// whether the version was newly created (true) or deduplicated (false).
func (s *Store) CreateVersion(
	projectID uuid.UUID,
	manifestContent string,
	lockContent string,
	description string,
) (*LocalProjectVersion, bool, error) {
	hash := contenthash.Hash(manifestContent, lockContent)

	// Dedup: if the latest version already has this hash, reuse it. Only
	// select the small scalar columns — the manifest/lock TEXT columns can
	// be large and we don't need them for the hash comparison.
	var latest LocalProjectVersion
	err := s.db.
		Select("id", "project_id", "version_number", "content_hash").
		Where("project_id = ?", projectID).
		Order("version_number DESC").
		First(&latest).Error
	if err == nil && latest.ContentHash == hash {
		return &latest, false, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, fmt.Errorf("checking latest version: %w", err)
	}

	v := &LocalProjectVersion{
		ProjectID:       projectID,
		ManifestContent: manifestContent,
		LockFileContent: lockContent,
		PackageMetadata: "[]",
		ContentHash:     hash,
		Description:     description,
		CreatedBy:       s.localUserID,
	}
	if err := s.db.Create(v).Error; err != nil {
		return nil, false, fmt.Errorf("creating version: %w", err)
	}
	return v, true, nil
}

// ListVersions returns all versions for a project, newest first. The
// large manifest/lock TEXT columns are omitted to keep the result cheap;
// callers that need them should use GetVersion.
func (s *Store) ListVersions(projectID uuid.UUID) ([]LocalProjectVersion, error) {
	var versions []LocalProjectVersion
	err := s.db.
		Select("id", "project_id", "version_number", "content_hash", "description", "created_by", "created_at").
		Where("project_id = ?", projectID).
		Order("version_number DESC").
		Find(&versions).Error
	if err != nil {
		return nil, fmt.Errorf("listing versions: %w", err)
	}
	return versions, nil
}

// GetVersion returns a single project version with full manifest and
// lock content. Returns nil if the version is not found.
func (s *Store) GetVersion(projectID uuid.UUID, versionNumber int) (*LocalProjectVersion, error) {
	var v LocalProjectVersion
	err := s.db.
		Where("project_id = ? AND version_number = ?", projectID, versionNumber).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting version: %w", err)
	}
	return &v, nil
}

// RollbackToVersion writes the target version's pixi.toml and pixi.lock
// to the project's directory and creates a new "Rolled back to snapshot N"
// snapshot. It does NOT run pixi install — callers should print a hint
// telling the user to run it themselves. Returns the newly-created
// rollback snapshot. If disk content already matches the rollback target,
// the existing latest version is returned (CreateVersion's dedup).
func (s *Store) RollbackToVersion(projectID uuid.UUID, versionNumber int) (*LocalProjectVersion, error) {
	target, err := s.GetVersion(projectID, versionNumber)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf("version %d not found", versionNumber)
	}

	project, err := s.GetProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("loading project: %w", err)
	}
	if project == nil {
		return nil, fmt.Errorf("project not found")
	}
	if project.Path == "" {
		return nil, fmt.Errorf("project has no path on disk")
	}

	manifestPath := filepath.Join(project.Path, "pixi.toml")
	if err := os.WriteFile(manifestPath, []byte(target.ManifestContent), 0644); err != nil {
		return nil, fmt.Errorf("writing pixi.toml: %w", err)
	}
	if target.LockFileContent != "" {
		lockPath := filepath.Join(project.Path, "pixi.lock")
		if err := os.WriteFile(lockPath, []byte(target.LockFileContent), 0644); err != nil {
			return nil, fmt.Errorf("writing pixi.lock: %w", err)
		}
	}

	description := fmt.Sprintf("Rolled back to snapshot %d", versionNumber)
	v, _, err := s.CreateVersion(projectID, target.ManifestContent, target.LockFileContent, description)
	if err != nil {
		return nil, fmt.Errorf("snapshotting rollback: %w", err)
	}
	return v, nil
}
