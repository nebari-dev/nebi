package local

// Owner: U3 (newversion). Create a version from a base version, tag
// semantics, change summary preview.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// CreateVersion validates the request and submits a JobTypeCreateVersion
// job with metadata {"base_version", "pixi_toml", "description", "tags"}.
func (s *Service) CreateVersion(ctx context.Context, projectID uuid.UUID, req shared.CreateVersionRequest) (*shared.JobSummary, error) {
	return nil, shared.NotImplemented("local.CreateVersion")
}

// RunCreateVersion is the create_version job body: solve the lock for the
// edited manifest and store version N+1 (BaseVersion, AuthorName, tags).
// It sets job.VersionNumber to the new number.
func (s *Service) RunCreateVersion(ctx context.Context, job *models.Job, log io.Writer) error {
	return shared.NotImplemented("local.RunCreateVersion")
}

// PreviewChanges compares a draft pixi.toml with its base version for the
// "Summary of changes" on the new version page.
func (s *Service) PreviewChanges(ctx context.Context, projectID uuid.UUID, baseVersion int, pixiToml string) (*shared.VersionDiff, error) {
	return nil, shared.NotImplemented("local.PreviewChanges")
}

// LatestTag is the tag that always points at the newest version.
const LatestTag = "latest"

// ApplyTags moves tags to version within tx: each tag is upserted in
// project_tags to point at version ("A tag stays with this version until a
// newer version uses the same tag"), and "latest" always moves to the
// highest version number of the project. Every code path that stores a
// version (create project, create version, pull, push on the server side
// mirrors it) must call it. Implemented by the foundation because U1, U3
// and U5 all store versions.
func ApplyTags(tx *gorm.DB, projectID uuid.UUID, version int, tags []string, userID uuid.UUID) error {
	upsert := func(tag string, n int) error {
		var existing models.ProjectTag
		err := tx.Where("project_id = ? AND tag = ?", projectID, tag).First(&existing).Error
		switch {
		case err == nil:
			if existing.VersionNumber == n {
				return nil
			}
			existing.VersionNumber = n
			return tx.Save(&existing).Error
		case errors.Is(err, gorm.ErrRecordNotFound):
			return tx.Create(&models.ProjectTag{ProjectID: projectID, Tag: tag, VersionNumber: n, CreatedBy: userID}).Error
		default:
			return err
		}
	}
	seen := map[string]bool{}
	for _, raw := range tags {
		tag := strings.TrimSpace(raw)
		if tag == "" || tag == LatestTag || seen[tag] {
			continue
		}
		seen[tag] = true
		if err := upsert(tag, version); err != nil {
			return fmt.Errorf("tag %q: %w", tag, err)
		}
	}
	var maxVersion struct{ Max *int }
	if err := tx.Model(&models.ProjectVersion{}).
		Select("MAX(version_number) AS max").
		Where("project_id = ?", projectID).
		Scan(&maxVersion).Error; err != nil {
		return err
	}
	latest := version
	if maxVersion.Max != nil && *maxVersion.Max > latest {
		latest = *maxVersion.Max
	}
	return upsert(LatestTag, latest)
}
