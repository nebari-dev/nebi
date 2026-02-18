package store

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Store manages the local nebi SQLite database via GORM.
type Store struct {
	db      *gorm.DB
	dataDir string
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

	// AutoMigrate workspace + config/credentials tables
	if err := db.AutoMigrate(&LocalWorkspace{}, &Config{}, &Credentials{}); err != nil {
		return nil, fmt.Errorf("migrating schema: %w", err)
	}

	// Seed singleton rows
	db.Exec("INSERT OR IGNORE INTO store_config (id) VALUES (1)")
	db.Exec("INSERT OR IGNORE INTO store_credentials (id) VALUES (1)")

	return &Store{db: db, dataDir: dataDir}, nil
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
type Config struct {
	ID        int    `gorm:"primarykey"`
	ServerURL string `gorm:"not null;default:''"`
}

func (Config) TableName() string { return "store_config" }

// Credentials stores auth info for the configured nebi server.
type Credentials struct {
	ID       int    `gorm:"primarykey"`
	Token    string `gorm:"not null;default:''"`
	Username string `gorm:"not null;default:''"`
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
