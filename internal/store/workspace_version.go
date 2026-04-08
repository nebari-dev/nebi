package store

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"gorm.io/gorm"
)

// CreateVersion creates a new workspace version snapshot. If the most recent
// version for the workspace has the same content hash, it is returned and a
// new record is NOT created (deduplication). The returned bool indicates
// whether the version was newly created (true) or deduplicated (false).
func (s *Store) CreateVersion(
	wsID uuid.UUID,
	manifestContent string,
	lockContent string,
	description string,
) (*LocalWorkspaceVersion, bool, error) {
	hash := contenthash.Hash(manifestContent, lockContent)

	// Dedup: if the latest version already has this hash, reuse it. Only
	// select the small scalar columns — the manifest/lock TEXT columns can
	// be large and we don't need them for the hash comparison.
	var latest LocalWorkspaceVersion
	err := s.db.
		Select("id", "workspace_id", "version_number", "content_hash").
		Where("workspace_id = ?", wsID).
		Order("version_number DESC").
		First(&latest).Error
	if err == nil && latest.ContentHash == hash {
		return &latest, false, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, fmt.Errorf("checking latest version: %w", err)
	}

	v := &LocalWorkspaceVersion{
		WorkspaceID:     wsID,
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
