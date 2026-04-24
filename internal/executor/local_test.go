package executor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
