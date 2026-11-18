package local

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// legacyMigrationID marks MigrateLegacyProjects as applied in client_migrations.
const legacyMigrationID = "legacy_projects_to_client_projects"

// LegacyProjectOrigin keeps the Origin* columns of a migrated project. Sync
// state moves to sdk/remote; until its ProjectRemote migration reads this
// table, the values are kept here instead of being dropped.
type LegacyProjectOrigin struct {
	ProjectID      uuid.UUID `gorm:"type:text;primaryKey" json:"project_id"`
	OriginID       string    `json:"origin_id"`
	OriginName     string    `json:"origin_name"`
	OriginTag      string    `json:"origin_tag"`
	OriginAction   string    `json:"origin_action"`
	OriginTomlHash string    `json:"origin_toml_hash"`
	OriginLockHash string    `json:"origin_lock_hash"`
}

// TableName is the holding table for legacy origin data.
func (LegacyProjectOrigin) TableName() string { return "legacy_project_origins" }

// clientMigration records one-time data migrations of the client database.
type clientMigration struct {
	ID        string `gorm:"primaryKey"`
	AppliedAt time.Time
}

func (clientMigration) TableName() string { return "client_migrations" }

// LegacyMigrationOptions configures MigrateLegacyProjects.
type LegacyMigrationOptions struct {
	// OldProjectsDir is the storage.projects_dir of the install that wrote
	// the old rows (for the desktop app, <app data dir>/projects). Paths
	// under it become ManagedDir. Old rows with an empty or relative path
	// were placed by nebi at <OldProjectsDir>/<normalized name>-<id>, so
	// OldProjectsDir is required when such rows exist.
	OldProjectsDir string
	// Logf receives one line per renamed, skipped or dropped row. Optional.
	Logf func(format string, args ...any)
}

// LegacyRename is a project renamed because its name was already used.
type LegacyRename struct {
	ProjectID uuid.UUID
	From, To  string
}

// LegacyDrop is an old row that was not migrated, and why.
type LegacyDrop struct {
	ProjectID  uuid.UUID
	Name, Path string
	Reason     string
}

// LegacyMigrationReport says what MigrateLegacyProjects did.
type LegacyMigrationReport struct {
	// AlreadyApplied is true when the migration had run before; nothing
	// else is set then.
	AlreadyApplied bool
	Migrated       int
	SkippedDeleted int
	Renamed        []LegacyRename
	Dropped        []LegacyDrop
	OriginsKept    int
}

type legacyRow struct {
	ID             string
	Name           string
	Path           string
	Status         string
	Source         string
	CreatedAt      *time.Time
	Deleted        bool
	OriginID       string
	OriginName     string
	OriginTag      string
	OriginAction   string
	OriginTomlHash string
	OriginLockHash string
}

func (r legacyRow) hasOrigin() bool {
	return r.OriginID != "" || r.OriginName != "" || r.OriginTag != "" ||
		r.OriginAction != "" || r.OriginTomlHash != "" || r.OriginLockHash != ""
}

// MigrateLegacyProjects copies the projects of the old shared "projects"
// table into client_projects, once. It runs in one transaction and records
// itself in client_migrations, so running it again does nothing.
//
//   - Soft-deleted rows, rows whose status is "deleting" and rows with an
//     invalid ID are dropped; the last two are reported. Status, Source,
//     SizeBytes and the other server columns are not copied.
//   - IDs and creation times are kept.
//   - Rows are taken oldest first. A name already used (ignoring case) gets
//     a "-2", "-3", ... suffix; each rename is logged and reported.
//   - Names that fail the new name rule are kept as they are; they must be
//     renamed before they can be pushed or published.
//   - A row whose directory is already used by an earlier row is dropped
//     and reported, since Path is unique.
//   - ManagedDir is true only for paths under OldProjectsDir whose source
//     is not "local" (a directory the user chose).
//   - Origin* values are kept in legacy_project_origins.
//
// Versions and tags are migrated by the version CRUD issue, after this. The
// old tables are left in place.
func MigrateLegacyProjects(ctx context.Context, db *gorm.DB, opts LegacyMigrationOptions) (*LegacyMigrationReport, error) {
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	oldDir := ""
	if opts.OldProjectsDir != "" {
		var err error
		if oldDir, err = normalizePath(opts.OldProjectsDir); err != nil {
			return nil, err
		}
	}

	if err := db.WithContext(ctx).AutoMigrate(&Project{}, &LegacyProjectOrigin{}, &clientMigration{}); err != nil {
		return nil, fmt.Errorf("migrating client tables: %w", err)
	}

	report := &LegacyMigrationReport{}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var applied int64
		if err := tx.Model(&clientMigration{}).Where("id = ?", legacyMigrationID).Count(&applied).Error; err != nil {
			return err
		}
		if applied > 0 {
			report.AlreadyApplied = true
			return nil
		}

		if tx.Migrator().HasTable("projects") {
			rows, err := readLegacyRows(tx)
			if err != nil {
				return err
			}
			if err := copyLegacyRows(tx, rows, oldDir, report, logf); err != nil {
				return err
			}
		}
		return tx.Create(&clientMigration{ID: legacyMigrationID, AppliedAt: time.Now()}).Error
	})
	if err != nil {
		return nil, fmt.Errorf("migrating legacy projects: %w", err)
	}
	return report, nil
}

// readLegacyRows reads the old projects table oldest first. Columns differ
// between databases written by the CLI store and by the server models, so
// a missing column reads as empty.
func readLegacyRows(tx *gorm.DB) ([]legacyRow, error) {
	m := tx.Migrator()
	col := func(name string) string {
		if m.HasColumn("projects", name) {
			return fmt.Sprintf("COALESCE(%s, '') AS %s", name, name)
		}
		return fmt.Sprintf("'' AS %s", name)
	}
	cols := []string{"id", "COALESCE(name, '') AS name", col("path"), col("status"), col("source")}
	order := "rowid ASC"
	if m.HasColumn("projects", "created_at") {
		cols = append(cols, "created_at")
		order = "created_at ASC, rowid ASC"
	} else {
		cols = append(cols, "NULL AS created_at")
	}
	if m.HasColumn("projects", "deleted_at") {
		cols = append(cols, "(deleted_at IS NOT NULL) AS deleted")
	} else {
		cols = append(cols, "0 AS deleted")
	}
	for _, c := range []string{"origin_id", "origin_name", "origin_tag", "origin_action", "origin_toml_hash", "origin_lock_hash"} {
		cols = append(cols, col(c))
	}

	var rows []legacyRow
	q := "SELECT " + strings.Join(cols, ", ") + " FROM projects ORDER BY " + order
	if err := tx.Raw(q).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("reading old projects: %w", err)
	}
	return rows, nil
}

func copyLegacyRows(tx *gorm.DB, rows []legacyRow, oldDir string, report *LegacyMigrationReport, logf func(string, ...any)) error {
	// Start from what client_projects already holds, so migrated rows never
	// collide with projects created there before the migration ran.
	var existing []Project
	if err := tx.Find(&existing).Error; err != nil {
		return err
	}
	ids := make(map[uuid.UUID]bool, len(existing))
	names := make(map[string]bool, len(existing))
	paths := make(map[string]uuid.UUID, len(existing))
	for _, p := range existing {
		ids[p.ID] = true
		names[strings.ToLower(p.Name)] = true
		paths[p.Path] = p.ID
	}

	for _, r := range rows {
		id, err := uuid.Parse(r.ID)
		if err != nil {
			reason := fmt.Sprintf("invalid id %q", r.ID)
			report.Dropped = append(report.Dropped, LegacyDrop{Name: r.Name, Path: r.Path, Reason: reason})
			logf("dropped project %q: %s", r.Name, reason)
			continue
		}
		if r.Deleted {
			report.SkippedDeleted++
			logf("skipped deleted project %s %q", id, r.Name)
			continue
		}
		if r.Status == legacyStatusDeleting {
			// A delete that never finished: the user asked for it to go.
			report.Dropped = append(report.Dropped, LegacyDrop{ProjectID: id, Name: r.Name, Path: r.Path, Reason: "was being deleted"})
			logf("dropped project %s %q: it was being deleted", id, r.Name)
			continue
		}
		if ids[id] {
			report.Dropped = append(report.Dropped, LegacyDrop{ProjectID: id, Name: r.Name, Path: r.Path, Reason: "already in client_projects"})
			logf("skipped project %s %q: already in client_projects", id, r.Name)
			continue
		}

		dir, err := legacyPath(r, id, oldDir)
		if err != nil {
			return err
		}
		if owner, taken := paths[dir]; taken {
			reason := fmt.Sprintf("directory %s is already used by project %s", dir, owner)
			report.Dropped = append(report.Dropped, LegacyDrop{ProjectID: id, Name: r.Name, Path: dir, Reason: reason})
			logf("dropped project %s %q: %s", id, r.Name, reason)
			continue
		}

		name := uniqueLegacyName(r.Name, names)
		if name != r.Name {
			report.Renamed = append(report.Renamed, LegacyRename{ProjectID: id, From: r.Name, To: name})
			logf("renamed project %s %q to %q: the name is already used", id, r.Name, name)
		}

		p := Project{
			ID:         id,
			Name:       name,
			Path:       dir,
			ManagedDir: oldDir != "" && isWithin(oldDir, dir) && r.Source != legacySourceLocal,
		}
		if r.CreatedAt != nil {
			p.CreatedAt = *r.CreatedAt
		}
		if err := tx.Create(&p).Error; err != nil {
			return fmt.Errorf("copying project %s %q: %w", id, r.Name, err)
		}
		ids[id] = true
		names[strings.ToLower(name)] = true
		paths[dir] = id
		report.Migrated++

		if r.hasOrigin() {
			origin := LegacyProjectOrigin{
				ProjectID:      id,
				OriginID:       r.OriginID,
				OriginName:     r.OriginName,
				OriginTag:      r.OriginTag,
				OriginAction:   r.OriginAction,
				OriginTomlHash: r.OriginTomlHash,
				OriginLockHash: r.OriginLockHash,
			}
			if err := tx.Create(&origin).Error; err != nil {
				return fmt.Errorf("keeping origin of project %s: %w", id, err)
			}
			report.OriginsKept++
		}
	}
	return nil
}

// Old column values the migration acts on.
const (
	// legacyStatusDeleting marks a row whose delete was started.
	legacyStatusDeleting = "deleting"
	// legacySourceLocal marks a project in a directory the user chose.
	legacySourceLocal = "local"
)

// legacyPath returns the clean, symlink-resolved directory of an old row.
// Rows without an absolute path live where the old local executor put them:
// <OldProjectsDir>/<normalized name>-<id>.
func legacyPath(r legacyRow, id uuid.UUID, oldDir string) (string, error) {
	p := strings.TrimSpace(r.Path)
	if p == "" || !filepath.IsAbs(p) {
		if oldDir == "" {
			return "", errors.New("old project " + id.String() + " has no absolute path; set LegacyMigrationOptions.OldProjectsDir to the old storage.projects_dir")
		}
		p = filepath.Join(oldDir, legacyDirName(r.Name)+"-"+id.String())
	}
	return resolveSymlinks(p)
}

var legacyDirNamePattern = regexp.MustCompile(`[^a-z0-9]+`)

// legacyDirName is the old local executor's directory-name normalization.
func legacyDirName(name string) string {
	n := legacyDirNamePattern.ReplaceAllString(strings.ToLower(name), "-")
	n = strings.Trim(n, "-")
	if len(n) > 50 {
		n = n[:50]
	}
	return n
}

// uniqueLegacyName returns name, or name-2, name-3, ... when the name is
// already used, ignoring case. An empty name becomes "project".
func uniqueLegacyName(name string, used map[string]bool) string {
	if strings.TrimSpace(name) == "" {
		name = "project"
	}
	if !used[strings.ToLower(name)] {
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if !used[strings.ToLower(candidate)] {
			return candidate
		}
	}
}
