package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/models"
)

// fakePixi writes a stub pixi that dumps its environment to dumpPath. The
// path is baked into the script rather than passed via an env var, because
// the allowlist would strip such a var before the child could read it.
func fakePixi(t *testing.T, dir, dumpPath string) string {
	t.Helper()
	path := filepath.Join(dir, "pixi")
	script := strings.ReplaceAll(`#!/bin/sh
if [ "$1" = "--version" ]; then echo "pixi 0.0.0-test"; exit 0; fi
env > DUMPPATH
exit 0
`, "DUMPPATH", dumpPath)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake pixi: %v", err)
	}
	return path
}

// newSandboxTestExecutor wires a LocalExecutor to a stub pixi that dumps its
// environment to dump, with the sandbox in "off" mode so only the env
// allowlist is exercised. It returns the executor and a workspace whose
// directory already exists.
func newSandboxTestExecutor(t *testing.T, dump string) (*LocalExecutor, *models.Workspace) {
	t.Helper()

	cfg := &config.Config{
		Mode:           "local",
		Storage:        config.StorageConfig{WorkspacesDir: t.TempDir()},
		PackageManager: config.PackageManagerConfig{DefaultType: "pixi", PixiPath: fakePixi(t, t.TempDir(), dump)},
		Sandbox:        config.SandboxConfig{Mode: "off"},
	}

	e, err := NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}

	ws := &models.Workspace{ID: uuid.New(), Name: "test-ws", PackageManager: "pixi"}
	if err := os.MkdirAll(e.GetWorkspacePath(ws), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	return e, ws
}

// TestListPackages_ScrubsSecretsFromBuildEnv guards the service-layer bypass:
// `pixi list` parses a user-supplied manifest and lockfile, so it must go
// through the same sandbox as a build rather than inheriting the server's
// environment.
func TestListPackages_ScrubsSecretsFromBuildEnv(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "env.txt")

	t.Setenv("NEBI_DATABASE_DSN", "host=db password=hunter2")
	t.Setenv("NEBI_AUTH_JWT_SECRET", "supersecret")

	e, ws := newSandboxTestExecutor(t, dump)

	if _, err := e.ListPackages(context.Background(), ws); err != nil {
		t.Fatalf("ListPackages: %v", err)
	}

	data, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("read env dump: %v", err)
	}
	got := string(data)
	for _, banned := range []string{"NEBI_DATABASE_DSN", "NEBI_AUTH_JWT_SECRET"} {
		if strings.Contains(got, banned) {
			t.Fatalf("secret %q reached the pixi list subprocess:\n%s", banned, got)
		}
	}
	if !strings.Contains(got, "HOME="+filepath.Join(e.GetWorkspacePath(ws), ".nebi-home")) {
		t.Fatalf("expected job-scoped HOME in pixi list env:\n%s", got)
	}
}

func TestInstallEnvironment_ScrubsSecretsFromBuildEnv(t *testing.T) {
	baseDir := t.TempDir()
	dump := filepath.Join(t.TempDir(), "env.txt")
	pixiPath := fakePixi(t, t.TempDir(), dump)

	t.Setenv("NEBI_DATABASE_DSN", "host=db password=hunter2")
	t.Setenv("NEBI_AUTH_JWT_SECRET", "supersecret")

	cfg := &config.Config{
		Mode:           "local",
		Storage:        config.StorageConfig{WorkspacesDir: baseDir},
		PackageManager: config.PackageManagerConfig{DefaultType: "pixi", PixiPath: pixiPath},
		Sandbox:        config.SandboxConfig{Mode: "off"},
	}

	e, err := NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}

	ws := &models.Workspace{ID: uuid.New(), Name: "test-ws", PackageManager: "pixi"}
	if err := os.MkdirAll(e.GetWorkspacePath(ws), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	var log strings.Builder
	if err := e.InstallEnvironment(context.Background(), ws, &log); err != nil {
		t.Fatalf("InstallEnvironment: %v (log: %s)", err, log.String())
	}

	data, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("read env dump: %v (log: %s)", err, log.String())
	}
	got := string(data)
	for _, banned := range []string{"NEBI_DATABASE_DSN", "NEBI_AUTH_JWT_SECRET"} {
		if strings.Contains(got, banned) {
			t.Fatalf("secret %q reached the build subprocess:\n%s", banned, got)
		}
	}
	if !strings.Contains(got, "HOME="+filepath.Join(e.GetWorkspacePath(ws), ".nebi-home")) {
		t.Fatalf("expected job-scoped HOME in build env:\n%s", got)
	}
}
