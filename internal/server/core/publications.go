package core

// Owner: U7 (publish). Publication metadata on the server (gap 2).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// RecordPublication stores publication metadata for a version: the
// registry host, repository, tag and digest a client published it to
// (never credentials). The client's publication ID is reused when given so
// the client can remove the record later; recording the same ID again
// updates it. Active records of the project with the same
// host/repository:tag are marked unpublished, because the tag now points
// at this version.
func (s *Service) RecordPublication(ctx context.Context, userID, projectID uuid.UUID, version int, rec PublicationRecord) (*PublicationRecord, error) {
	rec.RegistryHost = strings.TrimSpace(rec.RegistryHost)
	rec.Repository = strings.Trim(strings.TrimSpace(rec.Repository), "/")
	rec.Tag = strings.TrimSpace(rec.Tag)
	switch {
	case rec.RegistryHost == "":
		return nil, &ValidationError{Field: "registry_host", Message: "registry_host is required"}
	case rec.Repository == "":
		return nil, &ValidationError{Field: "repository", Message: "repository is required"}
	case rec.Tag == "":
		return nil, &ValidationError{Field: "tag", Message: "tag is required"}
	case strings.ContainsAny(rec.RegistryHost+rec.Repository+rec.Tag, "@ \t\n"):
		return nil, &ValidationError{Field: "repository", Message: "registry_host, repository and tag must be plain names"}
	}
	db := s.deps.DB.WithContext(ctx)
	if err := s.requireVersion(db, projectID, version); err != nil {
		return nil, err
	}

	publishedBy := rec.PublishedBy
	if publishedBy == "" && userID != uuid.Nil {
		var u models.User
		if err := db.Select("username").Where("id = ?", userID).First(&u).Error; err == nil {
			publishedBy = u.Username
		}
	}
	publishedAt := time.Now()
	if rec.PublishedAt != nil && !rec.PublishedAt.IsZero() {
		publishedAt = *rec.PublishedAt
	}

	row := models.VersionPublication{
		ProjectID:     projectID,
		VersionNumber: version,
		RegistryHost:  rec.RegistryHost,
		Repository:    rec.Repository,
		Tag:           rec.Tag,
		Digest:        rec.Digest,
		PublishedBy:   publishedBy,
		PublishedAt:   publishedAt,
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		var existing models.VersionPublication
		found := false
		if rec.ID != nil && *rec.ID != uuid.Nil {
			err := tx.Where("id = ?", *rec.ID).First(&existing).Error
			switch {
			case err == nil && existing.ProjectID == projectID:
				found = true
			case err == nil:
				// The ID belongs to another project; never touch it.
			case errors.Is(err, gorm.ErrRecordNotFound):
				row.ID = *rec.ID
			default:
				return err
			}
		}
		now := time.Now()
		q := tx.Model(&models.VersionPublication{}).
			Where("project_id = ? AND registry_host = ? AND repository = ? AND tag = ? AND unpublished_at IS NULL",
				projectID, row.RegistryHost, row.Repository, row.Tag)
		if found {
			q = q.Where("id <> ?", existing.ID)
		}
		if err := q.Update("unpublished_at", now).Error; err != nil {
			return err
		}
		if found {
			row.ID = existing.ID
			row.CreatedAt = existing.CreatedAt
			return tx.Model(&models.VersionPublication{}).Where("id = ?", existing.ID).Updates(map[string]any{
				"version_number": row.VersionNumber,
				"registry_host":  row.RegistryHost,
				"repository":     row.Repository,
				"tag":            row.Tag,
				"digest":         row.Digest,
				"published_by":   row.PublishedBy,
				"published_at":   row.PublishedAt,
				"unpublished_at": nil,
			}).Error
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return nil, fmt.Errorf("recording publication: %w", err)
	}
	out := publicationRecord(row)
	return &out, nil
}

// RemovePublication marks a publication unpublished. Removing an already
// unpublished record is not an error.
func (s *Service) RemovePublication(ctx context.Context, userID, projectID uuid.UUID, version int, publicationID uuid.UUID) error {
	db := s.deps.DB.WithContext(ctx)
	var row models.VersionPublication
	err := db.Where("id = ? AND project_id = ? AND version_number = ?", publicationID, projectID, version).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("publication %s: %w", publicationID, ErrNotFound)
	}
	if err != nil {
		return err
	}
	if row.UnpublishedAt != nil {
		return nil
	}
	return db.Model(&models.VersionPublication{}).Where("id = ?", row.ID).Update("unpublished_at", time.Now()).Error
}

// PublicationsByVersion returns active publication records keyed by
// version number (used by ListSyncVersions), oldest first.
func (s *Service) PublicationsByVersion(ctx context.Context, projectID uuid.UUID) (map[int][]PublicationRecord, error) {
	var rows []models.VersionPublication
	if err := s.deps.DB.WithContext(ctx).
		Where("project_id = ? AND unpublished_at IS NULL", projectID).
		Order("published_at, created_at").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing publications: %w", err)
	}
	out := make(map[int][]PublicationRecord, len(rows))
	for _, r := range rows {
		out[r.VersionNumber] = append(out[r.VersionNumber], publicationRecord(r))
	}
	return out, nil
}

// requireVersion fails with ErrNotFound unless the project has the version.
func (s *Service) requireVersion(db *gorm.DB, projectID uuid.UUID, version int) error {
	var count int64
	if err := db.Model(&models.ProjectVersion{}).
		Where("project_id = ? AND version_number = ?", projectID, version).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("version %d: %w", version, ErrNotFound)
	}
	return nil
}

func publicationRecord(r models.VersionPublication) PublicationRecord {
	id := r.ID
	at := r.PublishedAt
	return PublicationRecord{
		ID:           &id,
		RegistryHost: r.RegistryHost,
		Repository:   r.Repository,
		Tag:          r.Tag,
		Digest:       r.Digest,
		PublishedBy:  r.PublishedBy,
		PublishedAt:  &at,
	}
}
