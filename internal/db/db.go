package db

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/aktech/darb/internal/config"
	"github.com/aktech/darb/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// New creates a new database connection based on configuration
func New(cfg config.DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch cfg.Driver {
	case "sqlite":
		// Configure SQLite with WAL mode and busy timeout for better concurrency
		dialector = sqlite.Open(cfg.DSN + "?_journal_mode=WAL&_busy_timeout=5000")
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
		// SQLite: Use single connection to avoid locking issues
		// WAL mode allows concurrent reads but only one writer
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		slog.Info("Configured SQLite with WAL mode and single connection")
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

// Migrate runs database migrations for all models
func Migrate(db *gorm.DB) error {
	slog.Info("Running database migrations...")

	// Auto-migrate all models
	err := db.AutoMigrate(
		&models.User{},
		&models.Role{},
		&models.Environment{},
		&models.Job{},
		&models.Permission{},
		&models.Template{},
		&models.Package{},
		&models.AuditLog{},
		&models.EnvironmentVersion{},
	)
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Seed default roles if they don't exist
	if err := seedDefaultRoles(db); err != nil {
		return fmt.Errorf("failed to seed default roles: %w", err)
	}

	return nil
}

// seedDefaultRoles creates default roles (admin, owner, editor, viewer)
func seedDefaultRoles(db *gorm.DB) error {
	defaultRoles := []models.Role{
		{Name: "admin", Description: "Full system access including user management"},
		{Name: "owner", Description: "Full access to owned environments"},
		{Name: "editor", Description: "Can modify environments but not delete"},
		{Name: "viewer", Description: "Read-only access to environments"},
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
