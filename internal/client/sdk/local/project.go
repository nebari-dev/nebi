package local

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Project is a pixi workspace on this machine that nebi tracks.
// The directory at Path holds the working copy (pixi.toml, pixi.lock,
// .pixi/ and any other project files). Versions are immutable snapshots
// of pixi.toml and pixi.lock stored in the database.
type Project struct {
	ID uuid.UUID `gorm:"type:text;primaryKey" json:"id"`

	// Name is the local display name. Unique case-insensitively, validated
	// by ValidateProjectName. Renaming does not touch files on disk.
	Name string `gorm:"not null;uniqueIndex:idx_projects_name,collate:NOCASE" json:"name"`

	// Path is the absolute, symlink-resolved project directory.
	Path string `gorm:"not null;uniqueIndex" json:"path"`

	// ManagedDir is true when nebi created Path under the default projects
	// directory. Only managed directories can be removed from disk by Delete.
	ManagedDir bool `gorm:"not null;default:false" json:"managed_dir"`

	// InstalledVersionID is the version whose environment is materialized in
	// Path/.pixi. Nil when nothing is installed. Written by sdk/exec.
	InstalledVersionID *uuid.UUID `gorm:"type:text" json:"installed_version_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// TableName keeps the client table apart from the server's "projects" table.
func (Project) TableName() string { return "client_projects" }

// BeforeCreate assigns an ID when none is set.
func (p *Project) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

// Migrate creates or updates the project table in the client database. It
// is idempotent.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&Project{}); err != nil {
		return fmt.Errorf("migrating client project table: %w", err)
	}
	return nil
}

func (s *Service) loadProject(ctx context.Context, id uuid.UUID) (*Project, error) {
	var p Project
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("loading project %s: %w", id, err)
	}
	return &p, nil
}

// trackedPath is the identity and directory of a tracked project.
type trackedPath struct {
	ID   uuid.UUID
	Name string
	Path string
}

// trackedPaths returns the directory of every tracked project.
func (s *Service) trackedPaths(ctx context.Context) ([]trackedPath, error) {
	var out []trackedPath
	if err := s.db.WithContext(ctx).Model(&Project{}).Select("id", "name", "path").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("reading project paths: %w", err)
	}
	return out, nil
}

// uniqueViolation maps a unique-index error from inserting or updating a
// project row to ErrNameTaken or ErrPathTaken. Other errors are returned
// unchanged. The check-then-write paths already return these errors; this
// covers a write that races with another process using the same database.
func uniqueViolation(err error, p *Project) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if !strings.Contains(msg, "UNIQUE constraint failed") {
		return err
	}
	switch {
	case strings.Contains(msg, "client_projects.name"):
		return fmt.Errorf("%w: %q", ErrNameTaken, p.Name)
	case strings.Contains(msg, "client_projects.path"):
		return fmt.Errorf("%w: %s", ErrPathTaken, p.Path)
	}
	return err
}
