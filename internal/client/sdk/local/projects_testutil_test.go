package local_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
)

// fakeLock is a minimal pixi.lock (format v6) the fake executor writes.
const fakeLock = `version: 6
environments:
  default:
    channels:
    - url: https://conda.anaconda.org/conda-forge/
    packages:
      linux-64:
      - conda: https://conda.anaconda.org/conda-forge/linux-64/python-3.12.1-h1234_0.conda
packages:
- conda: https://conda.anaconda.org/conda-forge/linux-64/python-3.12.1-h1234_0.conda
  sha256: 0000
  md5: 0000
  size: 1000
`

// fakeExecutor stands in for pixi: CreateProject writes pixi.toml and a
// canned lock, InstallEnvironment creates .pixi/envs.
type fakeExecutor struct {
	mu        sync.Mutex
	failSolve bool
	created   []string
	deleted   []string
}

var _ executor.Executor = (*fakeExecutor)(nil)

func (f *fakeExecutor) CreateProject(ctx context.Context, p *models.Project, log io.Writer, opts executor.CreateProjectOptions) error {
	dir := f.GetProjectPath(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte(opts.PixiToml), 0o644); err != nil {
		return err
	}
	f.mu.Lock()
	f.created = append(f.created, dir)
	fail := f.failSolve
	f.mu.Unlock()
	fmt.Fprintln(log, "Running: pixi lock")
	if fail {
		return errors.New("failed to lock pixi environment: exit status 1")
	}
	return os.WriteFile(filepath.Join(dir, "pixi.lock"), []byte(fakeLock), 0o644)
}

func (f *fakeExecutor) InstallPackages(context.Context, *models.Project, []string, io.Writer) error {
	return nil
}

func (f *fakeExecutor) RemovePackages(context.Context, *models.Project, []string, io.Writer) error {
	return nil
}

func (f *fakeExecutor) DeleteProject(ctx context.Context, p *models.Project, log io.Writer) error {
	if p.Source == "local" {
		return nil
	}
	f.mu.Lock()
	f.deleted = append(f.deleted, f.GetProjectPath(p))
	f.mu.Unlock()
	return os.RemoveAll(f.GetProjectPath(p))
}

func (f *fakeExecutor) SolveEnvironment(context.Context, *models.Project, io.Writer) error {
	return nil
}

func (f *fakeExecutor) CleanupJobArtifacts(context.Context, *models.Project, models.JobType, io.Writer) error {
	return nil
}

func (f *fakeExecutor) InstallEnvironment(ctx context.Context, p *models.Project, log io.Writer) error {
	envs := filepath.Join(f.GetProjectPath(p), ".pixi", "envs", "default")
	if err := os.MkdirAll(envs, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(envs, "python"), make([]byte, 4096), 0o644)
}

func (f *fakeExecutor) UninstallEnvironment(ctx context.Context, p *models.Project, log io.Writer) error {
	return os.RemoveAll(filepath.Join(f.GetProjectPath(p), ".pixi", "envs"))
}

func (f *fakeExecutor) IsEnvInstalled(p *models.Project) bool {
	info, err := os.Stat(filepath.Join(f.GetProjectPath(p), ".pixi", "envs"))
	return err == nil && info.IsDir()
}

func (f *fakeExecutor) GetProjectPath(p *models.Project) string { return p.Path }

func (f *fakeExecutor) StagingRoot() string { return os.TempDir() }

// newTestStack builds the client stack with inline jobs (CLI style) over a
// fresh database and the fake executor.
func newTestStack(t *testing.T) (*client.Stack, *fakeExecutor, string) {
	t.Helper()
	dataDir := t.TempDir()
	gdb, err := local.OpenDB(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	fake := &fakeExecutor{}
	projectsDir := filepath.Join(dataDir, "projects")
	stack, err := client.NewStack(client.StackOptions{
		DB:          gdb,
		Executor:    fake,
		DataDir:     dataDir,
		ProjectsDir: projectsDir,
		InlineLog:   io.Discard,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return stack, fake, projectsDir
}
