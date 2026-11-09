package client

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/nebari-dev/nebi/internal/client/desktop"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/store"
)

// OpenCLI opens the local database in the Nebi data dir (NEBI_DATA_DIR or
// store.DefaultDataDir()), migrates it and builds a stack whose jobs run
// inline with their log written to logOut. New projects default to
// <data dir>/projects (NEBI_STORAGE_PROJECTS_DIR overrides it). Call the
// returned close function when done.
func OpenCLI(logOut io.Writer) (*Stack, func() error, error) {
	dataDir, err := store.DefaultDataDir()
	if err != nil {
		return nil, nil, fmt.Errorf("determining data directory: %w", err)
	}
	gdb, err := local.OpenDB(dataDir)
	if err != nil {
		return nil, nil, err
	}
	closeDB := func() error {
		sqlDB, err := gdb.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	if err := local.Migrate(gdb); err != nil {
		_ = closeDB()
		return nil, nil, fmt.Errorf("migrating local database: %w", err)
	}

	cfg, err := config.Load(config.WithMode(config.ModeLocal))
	if err != nil {
		_ = closeDB()
		return nil, nil, fmt.Errorf("loading configuration: %w", err)
	}
	projectsDir := filepath.Join(dataDir, "projects")
	if v := os.Getenv("NEBI_STORAGE_PROJECTS_DIR"); v != "" {
		projectsDir = v
	}
	cfg.Storage.ProjectsDir = projectsDir
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		_ = closeDB()
		return nil, nil, fmt.Errorf("initializing executor: %w", err)
	}
	stack, err := NewStack(StackOptions{
		DB:          gdb,
		Executor:    exec,
		DataDir:     dataDir,
		ProjectsDir: projectsDir,
		InlineLog:   logOut,
		Desktop:     desktop.Capabilities{},
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		_ = closeDB()
		return nil, nil, err
	}
	return stack, closeDB, nil
}
