package store

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Store manages the local nebi SQLite database via GORM.
type Store struct {
	db          *gorm.DB
	dataDir     string
	localUserID uuid.UUID
}

// New creates a Store using the default platform data directory.
func New() (*Store, error) {
	dataDir, err := DefaultDataDir()
	if err != nil {
		return nil, fmt.Errorf("determining data directory: %w", err)
	}
	return Open(dataDir)
}

// Open creates a Store with a specific data directory.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "nebi.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode
	db.Exec("PRAGMA journal_mode=WAL")

	if err := MigrateLocalTables(db); err != nil {
		return nil, err
	}

	// Ensure the well-known local-user row exists so that FK constraints
	// added later by the server migration (project_versions.created_by
	// -> users.id) are satisfied by records the CLI creates.
	localUserID, err := ensureLocalUser(db)
	if err != nil {
		return nil, fmt.Errorf("seeding local user: %w", err)
	}

	return &Store{db: db, dataDir: dataDir, localUserID: localUserID}, nil
}

// MigrateLocalTables creates the CLI store tables (legacy Local* mirrors,
// store_config, store_credentials) on db and seeds the singleton rows. It is
// called by Open and by internal/client/sdk/local.Migrate, which owns the
// local database schema for the CLI, nebi-web and the desktop app.
func MigrateLocalTables(db *gorm.DB) error {
	// AutoMigrate project + config/credentials tables
	if err := db.AutoMigrate(&LocalUser{}, &LocalProject{}, &LocalProjectVersion{}, &Config{}, &Credentials{}, &LocalRegistry{}, &LocalPublication{}); err != nil {
		return fmt.Errorf("migrating schema: %w", err)
	}

	// Drop the legacy package_manager column: pixi is the only package
	// manager, and the column was NOT NULL so leaving it would break inserts
	// in databases created before its removal.
	if db.Migrator().HasColumn(&LocalProject{}, "package_manager") {
		if err := db.Migrator().DropColumn(&LocalProject{}, "package_manager"); err != nil {
			return fmt.Errorf("dropping projects.package_manager column: %w", err)
		}
	}

	// Seed singleton rows
	db.Where(Config{ID: 1}).FirstOrCreate(&Config{ID: 1})
	db.Where(Credentials{ID: 1}).FirstOrCreate(&Credentials{ID: 1})
	return nil
}

// DB returns the underlying GORM DB for advanced queries.
func (s *Store) DB() *gorm.DB {
	return s.db
}

// DataDir returns the store's data directory.
func (s *Store) DataDir() string {
	return s.dataDir
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// DefaultDataDir returns ~/.local/share/nebi/ on Linux, platform equivalent elsewhere.
func DefaultDataDir() (string, error) {
	if dir := os.Getenv("NEBI_DATA_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "nebi"), nil
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			return filepath.Join(appData, "nebi"), nil
		}
		return filepath.Join(home, "AppData", "Roaming", "nebi"), nil
	default:
		return filepath.Join(home, ".local", "share", "nebi"), nil
	}
}

// Config is a singleton table for store configuration (server URL).
// ServerName and LastSyncedAt back the client Server page (issue #649):
// the display name of the connected server and when its project list was
// last refreshed.
type Config struct {
	ID           int    `gorm:"primarykey"`
	ServerURL    string `gorm:"not null;default:''"`
	ServerName   string `gorm:"not null;default:''"`
	LastSyncedAt *time.Time
}

func (Config) TableName() string { return "store_config" }

// Credentials stores auth info for the configured nebi server. AuthMode is
// "none" or "token" ("" is treated as "token" when Token is set).
type Credentials struct {
	ID       int    `gorm:"primarykey"`
	Token    string `gorm:"not null;default:''"`
	Username string `gorm:"not null;default:''"`
	AuthMode string `gorm:"not null;default:''"`
}

func (Credentials) TableName() string { return "store_credentials" }

// MigrateServerDB creates store tables on an external DB (used by local-mode server).
func MigrateServerDB(db *gorm.DB) error {
	if err := db.AutoMigrate(&Config{}, &Credentials{}); err != nil {
		return fmt.Errorf("migrating store tables: %w", err)
	}
	// Seed singleton rows using GORM (dialect-agnostic)
	db.Where(Config{ID: 1}).FirstOrCreate(&Config{ID: 1})
	db.Where(Credentials{ID: 1}).FirstOrCreate(&Credentials{ID: 1})
	return nil
}
