package local

// Owner: U2 (detail). Version list and files of local versions.

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

// ListVersions returns the project's versions newest first (author,
// description, tags without content-hash tags, base version, installed),
// then runs the registered VersionDecorators (published badges, sync
// markers, server-only versions).
func (s *Service) ListVersions(ctx context.Context, projectID uuid.UUID) ([]shared.VersionItem, error) {
	return nil, shared.NotImplemented("local.ListVersions")
}

// GetVersion returns one local version with its environment spec.
func (s *Service) GetVersion(ctx context.Context, projectID uuid.UUID, number int) (*shared.VersionDetail, error) {
	return nil, shared.NotImplemented("local.GetVersion")
}

// LoadVersion returns the stored version row (manifest and lock included).
// Returns shared.ErrNotFound when the version is not on this machine.
// Implemented by the foundation because several units use it.
func (s *Service) LoadVersion(ctx context.Context, projectID uuid.UUID, number int) (*models.ProjectVersion, error) {
	var v models.ProjectVersion
	err := s.deps.DB.WithContext(ctx).
		Where("project_id = ? AND version_number = ?", projectID, number).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("version %d: %w", number, shared.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// IsContentHashTag reports whether tag is an internal content-hash tag
// ("sha-" + 12 hex), which VersionItem.Tags never shows.
func IsContentHashTag(tag string) bool {
	if len(tag) != 16 || !strings.HasPrefix(tag, "sha-") {
		return false
	}
	for _, r := range tag[4:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// GetVersionFile returns "pixi.toml" or "pixi.lock" of a version.
func (s *Service) GetVersionFile(ctx context.Context, projectID uuid.UUID, number int, name string) (*shared.FileContent, error) {
	return nil, shared.NotImplemented("local.GetVersionFile")
}

// ResolvedPackages lists resolved packages per platform from pixi.lock.
func (s *Service) ResolvedPackages(ctx context.Context, projectID uuid.UUID, number int) (*shared.ResolvedPackages, error) {
	return nil, shared.NotImplemented("local.ResolvedPackages")
}

// WriteVersionArchive writes a .tar.gz of the version's files to w and
// returns the suggested file name.
func (s *Service) WriteVersionArchive(ctx context.Context, projectID uuid.UUID, number int, w io.Writer) (string, error) {
	return "", shared.NotImplemented("local.WriteVersionArchive")
}
