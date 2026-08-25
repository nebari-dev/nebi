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

// assertOffModeHome checks the off-mode half of the HOME contract: the
// parent's own HOME survives and no job-scoped home is planted in the
// workspace. Redirecting HOME here would give the build nothing it could not
// already reach (it is unconfined in off mode) while dropping a multi-GB
// package cache into what is, for local workspaces, the user's own project
// directory, and hiding $HOME/.rattler/credentials.json from pixi.
//
// TMPDIR is not part of this contract: internal/process scopes it into the
// workspace for every build. The active-mode half is pinned in
// internal/sandbox.
func assertOffModeHome(t *testing.T, env, wsPath, parentHome string) {
	t.Helper()

	scoped := filepath.Join(wsPath, ".nebi", "home")
	if strings.Contains(env, "HOME="+scoped) {
		t.Fatalf("off mode must not redirect HOME into the workspace:\n%s", env)
	}
	if !strings.Contains(env, "HOME="+parentHome) {
		t.Fatalf("expected the parent HOME %q to pass through in off mode:\n%s", parentHome, env)
	}
	if _, err := os.Stat(scoped); !os.IsNotExist(err) {
		t.Fatalf("off mode must not create .nebi/home in the workspace (err=%v)", err)
	}
}

// TestListPackages_ScrubsSecretsFromBuildEnv guards the service-layer bypass:
// `pixi list` parses a user-supplied manifest and lockfile, so it must go
// through the same sandbox as a build rather than inheriting the server's
// environment.
func TestListPackages_ScrubsSecretsFromBuildEnv(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "env.txt")

	parentHome := t.TempDir()
	t.Setenv("HOME", parentHome)
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
	assertOffModeHome(t, got, e.GetWorkspacePath(ws), parentHome)
}

func TestInstallEnvironment_ScrubsSecretsFromBuildEnv(t *testing.T) {
	baseDir := t.TempDir()
	dump := filepath.Join(t.TempDir(), "env.txt")
	pixiPath := fakePixi(t, t.TempDir(), dump)

	parentHome := t.TempDir()
	t.Setenv("HOME", parentHome)
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
	assertOffModeHome(t, got, e.GetWorkspacePath(ws), parentHome)
}
