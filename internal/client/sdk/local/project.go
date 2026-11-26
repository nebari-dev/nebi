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
// .pixi/ and any other project files). Versions are immutable environment
// specifications (pixi.toml and pixi.lock) stored in the database.
type Project struct {
	ID uuid.UUID `gorm:"type:text;primaryKey" json:"id"`

	// Name is the local display name. Unique case-insensitively, validated
	// by ValidateProjectName. Renaming does not touch files on disk.
	Name string `gorm:"not null;uniqueIndex:idx_client_projects_name,collate:NOCASE" json:"name"`

	// Path is the absolute, symlink-resolved project directory.
	Path string `gorm:"not null;uniqueIndex" json:"path"`

	// ManagedDir is true when nebi created Path under the default projects
	// directory. Only managed directories can be removed from disk by Delete.
	ManagedDir bool `gorm:"not null;default:false" json:"managed_dir"`

	// InstalledVersionID is the version whose environment is materialized in
	// Path/.pixi. Nil when nothing is installed. Written only after an
	// install succeeds, by installStep (run by the create, create_version
	// and install jobs). CreateResolvedVersion never sets it. State is
	// installed only when it is set and .pixi/envs exists. A reinstall or
	// uninstall must clear it before touching .pixi, so a crash midway does
	// not leave the old version reported as installed, and must do so
	// inside its job body, never before Submit, so a read that sees it
	// cleared also sees the job (see readProjects).
	InstalledVersionID *uuid.UUID `gorm:"type:text" json:"installed_version_id,omitempty"`

	// LastInstalledVersionID is the version most recently installed with
	// success. It is kept when the project is uninstalled, so the projects
	// list can offer to install it again. installStep sets it together with
	// InstalledVersionID; nothing else writes it.
	LastInstalledVersionID *uuid.UUID `gorm:"type:text" json:"last_installed_version_id,omitempty"`

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

// Migrate creates the project tables in the client database:
// client_projects, client_project_versions, client_project_tags,
// client_pending_removals, where DeleteProject records the directories it
// is removing, and client_pending_restores, where an in-place solve records
// the files it must put back. It is idempotent.
//
// It only creates what is missing: tables, then columns and indexes of
// existing tables. It never alters or rebuilds an existing table, so it
// copies no rows and stays cheap however many versions are stored. GORM's
// AutoMigrate is not used because, with this SQLite driver, it reads every
// nullable column back as NOT NULL and rebuilds the table, copying all of
// its rows, on every run. Changing an existing column is not supported.
func Migrate(db *gorm.DB) error {
	// In foreign-key order: versions reference projects, tags reference both.
	for _, model := range []any{&Project{}, &ProjectVersion{}, &ProjectTag{}, &pendingRemoval{}, &pendingRestore{}} {
		if err := ensureTable(db, model); err != nil {
			return fmt.Errorf("migrating client project tables: %w", err)
		}
	}
	return nil
}

// ensureTable creates model's table when it is missing, or else adds the
// columns and indexes it lacks.
func ensureTable(db *gorm.DB, model any) error {
	m := db.Migrator()
	if !m.HasTable(model) {
		return m.CreateTable(model)
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return err
	}
	for _, name := range stmt.Schema.DBNames {
		if field := stmt.Schema.LookUpField(name); field == nil || field.IgnoreMigration {
			continue
		}
		if !m.HasColumn(model, name) {
			if err := m.AddColumn(model, name); err != nil {
				return err
			}
		}
	}
	for _, idx := range stmt.Schema.ParseIndexes() {
		if !m.HasIndex(model, idx.Name) {
			if err := m.CreateIndex(model, idx.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) loadProject(ctx context.Context, id uuid.UUID) (*Project, error) {
	var p Project
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: project %s", ErrNotFound, id)
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
