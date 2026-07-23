package executor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/models"
)

func testExecutor(t *testing.T) *LocalExecutor {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{WorkspacesDir: dir},
	}
	exec, err := NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}
	return exec
}

func TestDeleteWorkspace_ManagedRemovesDir(t *testing.T) {
	exec := testExecutor(t)

	ws := &models.Workspace{
		ID:             uuid.New(),
		Name:           "managed-ws",
		Source:         "managed",
		PackageManager: "pixi",
	}

	// Create the directory that the executor would manage
	wsPath := exec.GetWorkspacePath(ws)
	if err := os.MkdirAll(wsPath, 0755); err != nil {
		t.Fatal(err)
	}
	// Put a marker file inside
	if err := os.WriteFile(filepath.Join(wsPath, "pixi.toml"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := exec.DeleteWorkspace(context.Background(), ws, &buf); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	if _, err := os.Stat(wsPath); !os.IsNotExist(err) {
		t.Fatalf("expected managed workspace directory to be removed, but it still exists")
	}
}

func TestDeleteWorkspace_LocalPreservesDir(t *testing.T) {
	exec := testExecutor(t)

	// Simulate a user's project directory
	userProjectDir := t.TempDir()
	markerFile := filepath.Join(userProjectDir, "pixi.toml")
	if err := os.WriteFile(markerFile, []byte("[project]\nname = \"my-project\""), 0644); err != nil {
		t.Fatal(err)
	}

	ws := &models.Workspace{
		ID:             uuid.New(),
		Name:           "local-ws",
		Source:         "local",
		Path:           userProjectDir,
		PackageManager: "pixi",
	}

	var buf bytes.Buffer
	if err := exec.DeleteWorkspace(context.Background(), ws, &buf); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	// The user's directory and files must still exist
	if _, err := os.Stat(userProjectDir); err != nil {
		t.Fatalf("expected local workspace directory to be preserved, got: %v", err)
	}
	if _, err := os.Stat(markerFile); err != nil {
		t.Fatalf("expected pixi.toml to be preserved, got: %v", err)
	}

	// Log should indicate skipping
	if !bytes.Contains(buf.Bytes(), []byte("skipping filesystem deletion")) {
		t.Errorf("expected log to mention skipping, got: %s", buf.String())
	}
}

func TestGetWorkspacePath_LocalReturnsUserPath(t *testing.T) {
	exec := testExecutor(t)

	userPath := "/home/user/my-project"
	ws := &models.Workspace{
		ID:     uuid.New(),
		Name:   "test",
		Source: "local",
		Path:   userPath,
	}

	got := exec.GetWorkspacePath(ws)
	if got != userPath {
		t.Errorf("expected %q, got %q", userPath, got)
	}
}

func TestGetWorkspacePath_ManagedReturnsDerivedPath(t *testing.T) {
	exec := testExecutor(t)

	ws := &models.Workspace{
		ID:     uuid.New(),
		Name:   "My Workspace",
		Source: "managed",
	}

	got := exec.GetWorkspacePath(ws)
	expected := filepath.Join(exec.baseDir, "my-workspace-"+ws.ID.String())
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestGetWorkspacePath_ManagedWithPersistedPathUsesPath(t *testing.T) {
	exec := testExecutor(t)

	persisted := filepath.Join(t.TempDir(), "existing-managed")
	ws := &models.Workspace{
		ID:     uuid.New(),
		Name:   "managed",
		Source: "managed",
		Path:   persisted,
	}

	got := exec.GetWorkspacePath(ws)
	if got != persisted {
		t.Errorf("expected persisted path %q, got %q", persisted, got)
	}
}

func TestGetWorkspacePath_LocalEmptyPathFallsBackToManaged(t *testing.T) {
	exec := testExecutor(t)

	ws := &models.Workspace{
		ID:     uuid.New(),
		Name:   "edge-case",
		Source: "local",
		Path:   "", // empty path should fall back to managed derivation
	}

	got := exec.GetWorkspacePath(ws)
	expected := filepath.Join(exec.baseDir, "edge-case-"+ws.ID.String())
	if got != expected {
		t.Errorf("expected managed fallback %q, got %q", expected, got)
	}
}

func TestDeleteWorkspace_EmptySourceRemovesDir(t *testing.T) {
	exec := testExecutor(t)

	ws := &models.Workspace{
		ID:     uuid.New(),
		Name:   "default-source",
		Source: "", // unset source should behave like managed
	}

	wsPath := exec.GetWorkspacePath(ws)
	if err := os.MkdirAll(wsPath, 0755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := exec.DeleteWorkspace(context.Background(), ws, &buf); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	if _, err := os.Stat(wsPath); !os.IsNotExist(err) {
		t.Fatalf("expected directory to be removed for empty-source workspace")
	}
}

func TestDeleteWorkspace_ManagedDirAlreadyGone(t *testing.T) {
	exec := testExecutor(t)

	ws := &models.Workspace{
		ID:     uuid.New(),
		Name:   "already-gone",
		Source: "managed",
	}

	// Don't create the directory — it's already missing
	var buf bytes.Buffer
	if err := exec.DeleteWorkspace(context.Background(), ws, &buf); err != nil {
		t.Fatalf("DeleteWorkspace on missing dir should not error, got: %v", err)
	}
}

func TestLocalExecutor_CreateWorkspace_SeedDirPopulatesWorkspace(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{WorkspacesDir: t.TempDir()},
		PackageManager: config.PackageManagerConfig{
			DefaultType: "pixi",
			PixiPath:    "/usr/bin/true", // no-op pixi install
		},
	}
	exec, err := NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}

	// Pre-stage an import directory with pixi + asset.
	stagingDir := t.TempDir()
	writeSeedFile(t, stagingDir, "pixi.toml", "[project]\nname = \"seed\"\n")
	writeSeedFile(t, stagingDir, "pixi.lock", "version: 6\n")
	writeSeedFile(t, stagingDir, "data/sample.csv", "a,b\n1,2\n")

	ws := &models.Workspace{
		ID:             uuid.New(),
		Name:           "seeded",
		PackageManager: "pixi",
	}

	var log bytes.Buffer
	err = exec.CreateWorkspace(context.Background(), ws, &log, CreateWorkspaceOptions{
		SeedDir: stagingDir,
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v\nlog: %s", err, log.String())
	}

	envPath := exec.GetWorkspacePath(ws)
	for rel, want := range map[string]string{
		"pixi.toml":       "[project]\nname = \"seed\"\n",
		"pixi.lock":       "version: 6\n",
		"data/sample.csv": "a,b\n1,2\n",
	} {
		got, err := os.ReadFile(filepath.Join(envPath, rel))
		if err != nil {
			t.Errorf("missing seeded file %s: %v", rel, err)
			continue
		}
		if string(got) != want {
			t.Errorf("seeded %s body mismatch:\ngot  %q\nwant %q", rel, got, want)
		}
	}

	// Staging dir should be gone after successful seed.
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Errorf("staging dir still exists after CreateWorkspace")
	}
}

// TestLocalExecutor_CreateWorkspace_SeedDirCleanedOnInstallFailure proves
// the staging dir is removed even when pixi install errors out — otherwise
// long-lived servers leak staging dirs on every failed import.
func TestLocalExecutor_CreateWorkspace_SeedDirCleanedOnInstallFailure(t *testing.T) {
	// Stub pixi: succeeds on `--version` (NewWithPath gate) but fails
	// on every other invocation (i.e. `install`).
	pixiBin := writeStubBinary(t, `#!/bin/sh
case "$1" in
  --version) echo "stub pixi 0.0.0"; exit 0 ;;
  *) exit 1 ;;
esac
`)

	cfg := &config.Config{
		Storage: config.StorageConfig{WorkspacesDir: t.TempDir()},
		PackageManager: config.PackageManagerConfig{
			DefaultType: "pixi",
			PixiPath:    pixiBin,
		},
	}
	exec, err := NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}

	stagingDir := t.TempDir()
	writeSeedFile(t, stagingDir, "pixi.toml", "[project]\nname = \"x\"\n")
	writeSeedFile(t, stagingDir, "pixi.lock", "version: 6\n")

	ws := &models.Workspace{ID: uuid.New(), Name: "fail-seed", PackageManager: "pixi"}
	var log bytes.Buffer
	err = exec.CreateWorkspace(context.Background(), ws, &log, CreateWorkspaceOptions{SeedDir: stagingDir})
	if err == nil {
		t.Fatalf("expected pixi install failure, got nil; log: %s", log.String())
	}

	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Errorf("staging dir leaked after install failure: stat err=%v\nlog: %s", err, log.String())
	}
}

// TestLocalExecutor_CreateWorkspace_PixiTomlRunsLockNotInstall proves the
// server-side create path only resolves the lockfile (pixi lock) and never
// downloads packages (pixi install).
func TestLocalExecutor_CreateWorkspace_PixiTomlRunsLockNotInstall(t *testing.T) {
	pixiBin, argsLog := writeRecordingStub(t)

	cfg := &config.Config{
		Storage: config.StorageConfig{WorkspacesDir: t.TempDir()},
		PackageManager: config.PackageManagerConfig{
			DefaultType: "pixi",
			PixiPath:    pixiBin,
		},
	}
	exec, err := NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}

	ws := &models.Workspace{ID: uuid.New(), Name: "lock-only", PackageManager: "pixi"}
	var log bytes.Buffer
	err = exec.CreateWorkspace(context.Background(), ws, &log, CreateWorkspaceOptions{
		PixiToml: "[project]\nname = \"lock-only\"\n",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v\nlog: %s", err, log.String())
	}

	calls := readStubCalls(t, argsLog)
	if !containsCall(calls, "lock") {
		t.Errorf("expected a `pixi lock` invocation, got calls: %v", calls)
	}
	if containsCall(calls, "install") {
		t.Errorf("expected no `pixi install` invocation, got calls: %v", calls)
	}
}

// TestLocalExecutor_SolveEnvironment_RunsLockNotInstall proves solving a
// manifest only refreshes the lockfile and never installs packages.
func TestLocalExecutor_SolveEnvironment_RunsLockNotInstall(t *testing.T) {
	pixiBin, argsLog := writeRecordingStub(t)

	cfg := &config.Config{
		Storage: config.StorageConfig{WorkspacesDir: t.TempDir()},
		PackageManager: config.PackageManagerConfig{
			DefaultType: "pixi",
			PixiPath:    pixiBin,
		},
	}
	exec, err := NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}

	ws := &models.Workspace{ID: uuid.New(), Name: "solve-lock", PackageManager: "pixi"}
	if err := os.MkdirAll(exec.GetWorkspacePath(ws), 0o755); err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer
	if err := exec.SolveEnvironment(context.Background(), ws, &log); err != nil {
		t.Fatalf("SolveEnvironment: %v\nlog: %s", err, log.String())
	}

	calls := readStubCalls(t, argsLog)
	if !containsCall(calls, "lock") {
		t.Errorf("expected a `pixi lock` invocation, got calls: %v", calls)
	}
	if containsCall(calls, "install") {
		t.Errorf("expected no `pixi install` invocation, got calls: %v", calls)
	}
}

// TestLocalExecutor_InstallEnvironment_RunsPixiInstall proves the explicit
// install step downloads packages from the existing lockfile.
func TestLocalExecutor_InstallEnvironment_RunsPixiInstall(t *testing.T) {
	pixiBin, argsLog := writeRecordingStub(t)

	cfg := &config.Config{
		Storage: config.StorageConfig{WorkspacesDir: t.TempDir()},
		PackageManager: config.PackageManagerConfig{
			DefaultType: "pixi",
			PixiPath:    pixiBin,
		},
	}
	exec, err := NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}

	ws := &models.Workspace{ID: uuid.New(), Name: "install-env", PackageManager: "pixi"}
	if err := os.MkdirAll(exec.GetWorkspacePath(ws), 0o755); err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer
	if err := exec.InstallEnvironment(context.Background(), ws, &log); err != nil {
		t.Fatalf("InstallEnvironment: %v\nlog: %s", err, log.String())
	}

	calls := readStubCalls(t, argsLog)
	if !containsCall(calls, "install") {
		t.Errorf("expected a `pixi install` invocation, got calls: %v", calls)
	}
}

// TestLocalExecutor_UninstallEnvironment_RemovesEnvsDir proves uninstall
// removes .pixi/envs while leaving manifest and lockfile in place, and
// that IsEnvInstalled tracks the transition.
func TestLocalExecutor_UninstallEnvironment_RemovesEnvsDir(t *testing.T) {
	exec := testExecutor(t)

	ws := &models.Workspace{ID: uuid.New(), Name: "uninstall-env", PackageManager: "pixi"}
	envPath := exec.GetWorkspacePath(ws)
	if err := os.MkdirAll(filepath.Join(envPath, ".pixi", "envs", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"pixi.toml", "pixi.lock"} {
		if err := os.WriteFile(filepath.Join(envPath, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if !exec.IsEnvInstalled(ws) {
		t.Fatalf("expected IsEnvInstalled=true with .pixi/envs present")
	}

	var log bytes.Buffer
	if err := exec.UninstallEnvironment(context.Background(), ws, &log); err != nil {
		t.Fatalf("UninstallEnvironment: %v", err)
	}

	if exec.IsEnvInstalled(ws) {
		t.Errorf("expected IsEnvInstalled=false after uninstall")
	}
	if _, err := os.Stat(filepath.Join(envPath, ".pixi", "envs")); !os.IsNotExist(err) {
		t.Errorf("expected .pixi/envs removed, stat err=%v", err)
	}
	for _, f := range []string{"pixi.toml", "pixi.lock"} {
		if _, err := os.Stat(filepath.Join(envPath, f)); err != nil {
			t.Errorf("expected %s preserved, got %v", f, err)
		}
	}
}

func TestLocalExecutor_IsEnvInstalled_FalseWithoutEnvs(t *testing.T) {
	exec := testExecutor(t)
	ws := &models.Workspace{ID: uuid.New(), Name: "no-envs", PackageManager: "pixi"}
	if err := os.MkdirAll(exec.GetWorkspacePath(ws), 0o755); err != nil {
		t.Fatal(err)
	}
	if exec.IsEnvInstalled(ws) {
		t.Errorf("expected IsEnvInstalled=false when .pixi/envs is absent")
	}
}

// TestLocalExecutor_PackageOps_UseNoInstall proves add/remove package
// operations only update manifest+lockfile (--no-install); materializing
// the environment stays an explicit step.
func TestLocalExecutor_PackageOps_UseNoInstall(t *testing.T) {
	pixiBin, argsLog := writeRecordingStub(t)

	cfg := &config.Config{
		Storage: config.StorageConfig{WorkspacesDir: t.TempDir()},
		PackageManager: config.PackageManagerConfig{
			DefaultType: "pixi",
			PixiPath:    pixiBin,
		},
	}
	exec, err := NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}

	ws := &models.Workspace{ID: uuid.New(), Name: "pkg-ops", PackageManager: "pixi"}
	if err := os.MkdirAll(exec.GetWorkspacePath(ws), 0o755); err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer
	if err := exec.InstallPackages(context.Background(), ws, []string{"numpy"}, &log); err != nil {
		t.Fatalf("InstallPackages: %v\nlog: %s", err, log.String())
	}
	if err := exec.RemovePackages(context.Background(), ws, []string{"numpy"}, &log); err != nil {
		t.Fatalf("RemovePackages: %v\nlog: %s", err, log.String())
	}

	calls := readStubCalls(t, argsLog)
	var sawAdd, sawRemove bool
	for _, c := range calls {
		if strings.HasPrefix(c, "add ") {
			sawAdd = true
			if !strings.Contains(c, "--no-install") {
				t.Errorf("pixi add missing --no-install: %q", c)
			}
		}
		if strings.HasPrefix(c, "remove ") {
			sawRemove = true
			if !strings.Contains(c, "--no-install") {
				t.Errorf("pixi remove missing --no-install: %q", c)
			}
		}
	}
	if !sawAdd || !sawRemove {
		t.Errorf("expected add and remove invocations, got calls: %v", calls)
	}
}

// writeRecordingStub writes a stub pixi binary that records every
// invocation's arguments to a log file. Returns (binaryPath, argsLogPath).
func writeRecordingStub(t *testing.T) (string, string) {
	t.Helper()
	argsLog := filepath.Join(t.TempDir(), "args.log")
	script := `#!/bin/sh
echo "$@" >> ` + argsLog + `
case "$1" in
  --version) echo "stub pixi 0.0.0" ;;
esac
exit 0
`
	return writeStubBinary(t, script), argsLog
}

// readStubCalls returns one entry per stub invocation (its argv joined by spaces).
func readStubCalls(t *testing.T, argsLog string) []string {
	t.Helper()
	data, err := os.ReadFile(argsLog)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var calls []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			calls = append(calls, line)
		}
	}
	return calls
}

// containsCall reports whether any recorded invocation's first argument
// (the pixi subcommand) equals subcommand.
func containsCall(calls []string, subcommand string) bool {
	for _, c := range calls {
		if strings.HasPrefix(c, subcommand+" ") || c == subcommand {
			return true
		}
	}
	return false
}

// writeStubBinary writes an executable shell script to a temp file and
// returns its path. Used to stub the pixi binary in tests.
func writeStubBinary(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeSeedFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeEnvName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"My Workspace", "my-workspace"},
		{"hello_world", "hello-world"},
		{"---leading---trailing---", "leading-trailing"},
		{"ALLCAPS", "allcaps"},
		{"special!@#$%^&*()chars", "special-chars"},
		{"a", "a"},
		{"", ""},
		{"already-clean", "already-clean"},
		{"multiple   spaces   here", "multiple-spaces-here"},
		// 60-char input should be truncated to 50
		{"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefgh",
			"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwx"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeEnvName(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeEnvName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
