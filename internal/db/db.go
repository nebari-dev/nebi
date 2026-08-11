package db

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// New creates a new database connection based on configuration
func New(cfg config.DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch cfg.Driver {
	case "sqlite":
		// Configure SQLite with PocketBase-inspired settings for production
		// Note: busy_timeout MUST be first before WAL mode is set
		pragmas := "?_pragma=busy_timeout(10000)" + // 10 seconds - wait for locks
			"&_pragma=journal_mode(WAL)" + // Write-Ahead Logging for concurrency
			"&_pragma=journal_size_limit(200000000)" + // 200MB WAL limit
			"&_pragma=synchronous(NORMAL)" + // Safe with WAL, faster than FULL
			"&_pragma=foreign_keys(ON)" + // Enforce referential integrity
			"&_pragma=temp_store(MEMORY)" + // Temp tables in RAM
			"&_pragma=cache_size(-32000)" // ~32MB cache
		dialector = sqlite.Open(cfg.DSN + pragmas)
	case "postgres", "postgresql":
		dialector = postgres.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	// Configure GORM logger (silent in production, info in dev)
	gormLogger := logger.Default.LogMode(logger.Info)

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormLogger,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	if cfg.Driver == "sqlite" {
		// SQLite with WAL mode can handle multiple concurrent connections
		// WAL allows many readers + one writer simultaneously
		// Using PocketBase-inspired settings: moderate pool for good concurrency
		sqlDB.SetMaxOpenConns(30)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxIdleTime(3 * time.Minute)
		slog.Info("Configured SQLite with WAL mode and connection pool",
			"max_open_conns", 30,
			"max_idle_conns", 5)
	} else if cfg.Driver == "postgres" || cfg.Driver == "postgresql" {
		// PostgreSQL: Use connection pool
		maxIdleConns := cfg.MaxIdleConns
		if maxIdleConns <= 0 {
			maxIdleConns = 10
		}
		maxOpenConns := cfg.MaxOpenConns
		if maxOpenConns <= 0 {
			maxOpenConns = 100
		}
		connMaxLifetime := cfg.ConnMaxLifetime
		if connMaxLifetime <= 0 {
			connMaxLifetime = 60 // Default 60 minutes
		}

		sqlDB.SetMaxIdleConns(maxIdleConns)
		sqlDB.SetMaxOpenConns(maxOpenConns)
		sqlDB.SetConnMaxLifetime(time.Duration(connMaxLifetime) * time.Minute)

		slog.Info("Configured PostgreSQL connection pool",
			"max_idle_conns", maxIdleConns,
			"max_open_conns", maxOpenConns,
			"conn_max_lifetime_min", connMaxLifetime)
	}

	return db, nil
}

// Migrate runs database migrations for all models. seedRegistry controls
// whether the built-in default registry is seeded (registries.seed_default).
func Migrate(db *gorm.DB, seedRegistry bool) error {
	slog.Info("Running database migrations...")

	// Auto-migrate all models
	err := db.AutoMigrate(
		&models.User{},
		&models.Role{},
		&models.Workspace{},
		&models.Job{},
		&models.Permission{},
		&models.Template{},
		&models.Package{},
		&models.AuditLog{},
		&models.WorkspaceVersion{},
		&models.OCIRegistry{},
		&models.Publication{},
		&models.WorkspaceTag{},
		&models.Group{},
		&models.GroupMember{},
		&models.GroupPermission{},
		&models.SystemSetting{},
	)
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Seed default roles if they don't exist
	if err := seedDefaultRoles(db); err != nil {
		return fmt.Errorf("failed to seed default roles: %w", err)
	}

	// Seed default registry (one-time; disabled via registries.seed_default)
	if seedRegistry {
		if err := seedDefaultRegistry(db); err != nil {
			return fmt.Errorf("failed to seed default registry: %w", err)
		}
	}

	return nil
}

// seedDefaultRoles creates default roles (admin, owner, editor, viewer)
func seedDefaultRoles(db *gorm.DB) error {
	defaultRoles := []models.Role{
		{Name: "admin", Description: "Full system access including user management"},
		{Name: "owner", Description: "Full access to owned workspaces"},
		{Name: "editor", Description: "Can modify workspaces but not delete"},
		{Name: "viewer", Description: "Read-only access to workspaces"},
	}

	for _, role := range defaultRoles {
		var existing models.Role
		result := db.Where("name = ?", role.Name).First(&existing)
		if result.Error == gorm.ErrRecordNotFound {
			if err := db.Create(&role).Error; err != nil {
				return err
			}
			slog.Info("Created default role", "role", role.Name)
		}
	}

	return nil
}

// defaultRegistrySeededKey marks that the built-in default registry has been
// seeded once. Without it the seed would resurrect a deliberately deleted
// default registry on every boot.
const defaultRegistrySeededKey = "default_registry_seeded"

// seedDefaultRegistry creates the default nebari-environments registry once.
// Databases that already have the row (created before the marker existed)
// get the marker backfilled without touching the row.
func seedDefaultRegistry(db *gorm.DB) error {
	var marker models.SystemSetting
	err := db.Where("key = ?", defaultRegistrySeededKey).First(&marker).Error
	if err == nil {
		return nil // seeded before; a missing row means it was deleted on purpose
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	var count int64
	db.Model(&models.OCIRegistry{}).Where("name = ?", "nebari-environments").Count(&count)
	if count == 0 {
		registry := models.OCIRegistry{
			Name:      "nebari-environments",
			URL:       "quay.io",
			Namespace: "nebari_environments",
			IsDefault: true,
		}
		// Concurrent boot (e.g. api + worker sharing Postgres in team mode)
		// can race this insert; let the DB's unique constraint silently
		// resolve the loser instead of crashing at boot.
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&registry).Error; err != nil {
			return err
		}
		slog.Info("Created default registry", "name", registry.Name, "url", registry.URL)
	}

	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.SystemSetting{Key: defaultRegistrySeededKey, Value: "true"}).Error
}
