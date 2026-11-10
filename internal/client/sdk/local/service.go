// Package local is the client SDK for state on this machine: projects,
// versions, tags, manifests, and the local database itself (issue #649).
//
// It is the single owner of local persistence. The CLI, nebi-web and the
// desktop app all use the same SQLite file, <data dir>/nebi.db, where the
// data dir is store.DefaultDataDir() (NEBI_DATA_DIR overrides it):
//   - CLI: OpenDB(dataDir) + Migrate
//   - desktop: app.go points NEBI_DATABASE_DSN at the same file
//   - nebi-web: internal/server.Run defaults the DSN to the same file in
//     local mode when NEBI_DATABASE_DSN is unset
//
// The schema is the server schema (internal/models via db.Migrate) plus the
// CLI store tables (store.MigrateLocalTables). New code reads and writes
// internal/models types; the store.Local* mirrors are legacy CLI views of
// the same tables.
//
// File ownership (see CONTRACTS.md): service.go is frozen; projects.go and
// manifest.go belong to U1, versions.go to U2, new_version.go to U3,
// server_credentials.go to U5, registry_credentials.go to U7.
package local

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/store"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Service is the local SDK. Methods are spread over unit-owned files.
type Service struct {
	deps shared.Deps
}

// New builds the local SDK.
func New(deps shared.Deps) *Service {
	return &Service{deps: deps}
}

// Deps exposes the dependencies to sibling SDK packages.
func (s *Service) Deps() shared.Deps { return s.deps }

// DB returns the local database.
func (s *Service) DB() *gorm.DB { return s.deps.DB }

// DBPath returns the local database file for a data dir.
func DBPath(dataDir string) string { return filepath.Join(dataDir, "nebi.db") }

// OpenDB opens (creating if needed) the local SQLite database in dataDir
// with the same pragmas as the server and a silent logger. Call Migrate
// before use.
func OpenDB(dataDir string) (*gorm.DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}
	pragmas := "?_pragma=busy_timeout(10000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)"
	gdb, err := gorm.Open(sqlite.Open(DBPath(dataDir)+pragmas), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w", err)
	}
	return gdb, nil
}

// Migrate brings a local database to the current schema. It is idempotent
// and safe to run on a database the server bootstrap already migrated.
func Migrate(gdb *gorm.DB) error {
	if err := db.Migrate(gdb, true); err != nil {
		return err
	}
	if err := store.MigrateLocalTables(gdb); err != nil {
		return err
	}
	return importLegacyCLIRegistries(gdb)
}

// importLegacyCLIRegistries copies registries the pre-#649 CLI kept in
// local_registries into oci_registries, which is the single local registry
// table from now on. Rows keep their ID; passwords stay in the keyring,
// keyed by registry name. Existing names are left alone.
func importLegacyCLIRegistries(gdb *gorm.DB) error {
	if !gdb.Migrator().HasTable(&store.LocalRegistry{}) {
		return nil
	}
	var legacy []store.LocalRegistry
	if err := gdb.Find(&legacy).Error; err != nil {
		return fmt.Errorf("reading legacy registries: %w", err)
	}
	for _, r := range legacy {
		var count int64
		if err := gdb.Model(&models.OCIRegistry{}).Where("name = ? OR id = ?", r.Name, r.ID).Count(&count).Error; err != nil {
			return fmt.Errorf("checking registry %q: %w", r.Name, err)
		}
		if count > 0 {
			continue
		}
		row := models.OCIRegistry{
			ID:        r.ID,
			Name:      r.Name,
			URL:       r.URL,
			Username:  r.Username,
			IsDefault: r.IsDefault,
			Namespace: r.Namespace,
		}
		if err := gdb.Create(&row).Error; err != nil {
			return fmt.Errorf("importing registry %q: %w", r.Name, err)
		}
	}
	return nil
}
