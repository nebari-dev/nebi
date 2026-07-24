package pixi

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/pkgmgr"
)

// TestPixiAvailable checks if pixi is available in PATH
func TestPixiAvailable(t *testing.T) {
	_, err := exec.LookPath("pixi")
	if err != nil {
		t.Skip("pixi not found in PATH, skipping pixi tests")
	}
}

// TestNew tests creating a new PixiManager
func TestNew(t *testing.T) {
	_, err := exec.LookPath("pixi")
	if err != nil {
		t.Skip("pixi not found in PATH, skipping test")
	}

	pm, err := New()
	if err != nil {
		t.Fatalf("Failed to create PixiManager: %v", err)
	}

	if pm.Name() != "pixi" {
		t.Errorf("Expected name 'pixi', got '%s'", pm.Name())
	}
}

// TestInit tests initializing a new pixi environment
func TestInit(t *testing.T) {
	_, err := exec.LookPath("pixi")
	if err != nil {
		t.Skip("pixi not found in PATH, skipping test")
	}

	pm, err := New()
	if err != nil {
		t.Fatalf("Failed to create PixiManager: %v", err)
	}

	// Create temporary directory for test environment
	tmpDir, err := os.MkdirTemp("", "nebi-pixi-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	opts := pkgmgr.InitOptions{
		EnvPath:  tmpDir,
		Name:     "test-env",
		Channels: []string{"conda-forge"},
	}

	err = pm.Init(ctx, opts)
	if err != nil {
		t.Fatalf("Failed to init environment: %v", err)
	}

	// Check that pixi.toml was created
	manifestPath := filepath.Join(tmpDir, "pixi.toml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Errorf("pixi.toml was not created at %s", manifestPath)
	}
}

// TestInstallAndList tests installing packages and listing them
func TestInstallAndList(t *testing.T) {
	_, err := exec.LookPath("pixi")
	if err != nil {
		t.Skip("pixi not found in PATH, skipping test")
	}

	pm, err := New()
	if err != nil {
		t.Fatalf("Failed to create PixiManager: %v", err)
	}

	// Create temporary directory for test environment
	tmpDir, err := os.MkdirTemp("", "nebi-pixi-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// Initialize environment
	initOpts := pkgmgr.InitOptions{
		EnvPath:  tmpDir,
		Name:     "test-env",
		Channels: []string{"conda-forge"},
	}
	if err := pm.Init(ctx, initOpts); err != nil {
		t.Fatalf("Failed to init environment: %v", err)
	}

	// Install a small package (python itself is usually quick)
	installOpts := pkgmgr.InstallOptions{
		EnvPath:  tmpDir,
		Packages: []string{"python=3.11"},
	}
	if err := pm.Install(ctx, installOpts); err != nil {
		t.Fatalf("Failed to install packages: %v", err)
	}

	// List packages
	listOpts := pkgmgr.ListOptions{
		EnvPath: tmpDir,
	}
	packages, err := pm.List(ctx, listOpts)
	if err != nil {
		t.Fatalf("Failed to list packages: %v", err)
	}

	// Check that python is in the list
	found := false
	for _, pkg := range packages {
		if pkg.Name == "python" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected to find 'python' in package list, got: %v", packages)
	}
}

// TestGetManifest tests reading and parsing the pixi.toml manifest
func TestGetManifest(t *testing.T) {
	t.Skip("Skipping flaky test - pixi init may not set project name in pixi.toml")

	_, err := exec.LookPath("pixi")
	if err != nil {
		t.Skip("pixi not found in PATH, skipping test")
	}

	pm, err := New()
	if err != nil {
		t.Fatalf("Failed to create PixiManager: %v", err)
	}

	// Create temporary directory for test environment
	tmpDir, err := os.MkdirTemp("", "nebi-pixi-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// Initialize environment
	initOpts := pkgmgr.InitOptions{
		EnvPath:  tmpDir,
		Name:     "test-manifest",
		Channels: []string{"conda-forge"},
	}
	if err := pm.Init(ctx, initOpts); err != nil {
		t.Fatalf("Failed to init environment: %v", err)
	}

	// Get manifest
	manifest, err := pm.GetManifest(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Failed to get manifest: %v", err)
	}

	// Check manifest fields
	if manifest.Name != "test-manifest" {
		t.Errorf("Expected manifest name 'test-manifest', got '%s'", manifest.Name)
	}

	if len(manifest.Channels) == 0 {
		t.Errorf("Expected at least one channel in manifest")
	}

	if len(manifest.Raw) == 0 {
		t.Errorf("Expected raw manifest content to be non-empty")
	}
}

// TestRemove tests removing packages
func TestRemove(t *testing.T) {
	_, err := exec.LookPath("pixi")
	if err != nil {
		t.Skip("pixi not found in PATH, skipping test")
	}

	pm, err := New()
	if err != nil {
		t.Fatalf("Failed to create PixiManager: %v", err)
	}

	// Create temporary directory for test environment
	tmpDir, err := os.MkdirTemp("", "nebi-pixi-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// Initialize environment
	initOpts := pkgmgr.InitOptions{
		EnvPath:  tmpDir,
		Name:     "test-env",
		Channels: []string{"conda-forge"},
	}
	if err := pm.Init(ctx, initOpts); err != nil {
		t.Fatalf("Failed to init environment: %v", err)
	}

	// Install a package
	installOpts := pkgmgr.InstallOptions{
		EnvPath:  tmpDir,
		Packages: []string{"python=3.11"},
	}
	if err := pm.Install(ctx, installOpts); err != nil {
		t.Fatalf("Failed to install packages: %v", err)
	}

	// Remove the package
	removeOpts := pkgmgr.RemoveOptions{
		EnvPath:  tmpDir,
		Packages: []string{"python"},
	}
	if err := pm.Remove(ctx, removeOpts); err != nil {
		t.Fatalf("Failed to remove package: %v", err)
	}

	// Verify package is removed
	listOpts := pkgmgr.ListOptions{
		EnvPath: tmpDir,
	}
	packages, err := pm.List(ctx, listOpts)
	if err != nil {
		t.Fatalf("Failed to list packages: %v", err)
	}

	// Check that python is NOT in the list
	for _, pkg := range packages {
		if pkg.Name == "python" {
			t.Errorf("Expected 'python' to be removed, but it's still in the list")
		}
	}
}

func TestPackageCommandsSeparateUserArgs(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *PixiManager, string) error
		want []string
	}{
		{
			name: "install",
			run: func(ctx context.Context, pm *PixiManager, envPath string) error {
				return pm.Install(ctx, pkgmgr.InstallOptions{
					EnvPath:  envPath,
					Packages: []string{"--config", "python=3.11"},
				})
			},
			want: []string{"add", "-v", "--", "--config", "python=3.11"},
		},
		{
			name: "install no-install",
			run: func(ctx context.Context, pm *PixiManager, envPath string) error {
				return pm.Install(ctx, pkgmgr.InstallOptions{
					EnvPath:   envPath,
					Packages:  []string{"--config", "python=3.11"},
					NoInstall: true,
				})
			},
			want: []string{"add", "-v", "--no-install", "--", "--config", "python=3.11"},
		},
		{
			name: "remove",
			run: func(ctx context.Context, pm *PixiManager, envPath string) error {
				return pm.Remove(ctx, pkgmgr.RemoveOptions{
					EnvPath:  envPath,
					Packages: []string{"--manifest-path", "python"},
				})
			},
			want: []string{"remove", "-v", "--", "--manifest-path", "python"},
		},
		{
			name: "remove no-install",
			run: func(ctx context.Context, pm *PixiManager, envPath string) error {
				return pm.Remove(ctx, pkgmgr.RemoveOptions{
					EnvPath:   envPath,
					Packages:  []string{"--manifest-path", "python"},
					NoInstall: true,
				})
			},
			want: []string{"remove", "-v", "--no-install", "--", "--manifest-path", "python"},
		},
		{
			name: "update",
			run: func(ctx context.Context, pm *PixiManager, envPath string) error {
				return pm.Update(ctx, pkgmgr.UpdateOptions{
					EnvPath:  envPath,
					Packages: []string{"--frozen", "python"},
				})
			},
			want: []string{"update", "--", "--frozen", "python"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm, argsPath, envPath := newRecordingPixi(t)

			if err := tt.run(context.Background(), pm, envPath); err != nil {
				t.Fatalf("command failed: %v", err)
			}

			got := readRecordedArgs(t, argsPath)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("recorded args = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestValidatePackageArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "valid package spec", args: []string{"python=3.11"}},
		{name: "valid option-like package", args: []string{"--config"}},
		{name: "empty string", args: []string{""}, wantErr: true},
		{name: "whitespace only", args: []string{" \t "}, wantErr: true},
		{name: "nul byte", args: []string{"python\x00numpy"}, wantErr: true},
		{name: "newline", args: []string{"python\nnumpy"}, wantErr: true},
		{name: "carriage return", args: []string{"python\rnumpy"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePackageArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validatePackageArgs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func newRecordingPixi(t *testing.T) (*PixiManager, string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("recording pixi test uses a POSIX shell script")
	}

	tmpDir := t.TempDir()
	pixiPath := filepath.Join(tmpDir, "pixi")
	script := `#!/bin/sh
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$0.args"
done
if [ "${1:-}" = "--version" ]; then
  exit 0
fi
exit 0
`
	if err := os.WriteFile(pixiPath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake pixi: %v", err)
	}

	pm, err := NewWithPath(pixiPath)
	if err != nil {
		t.Fatalf("create PixiManager with fake pixi: %v", err)
	}

	argsPath := pixiPath + ".args"
	if err := os.WriteFile(argsPath, nil, 0644); err != nil {
		t.Fatalf("clear recorded args: %v", err)
	}

	return pm, argsPath, tmpDir
}

func readRecordedArgs(t *testing.T, argsPath string) []string {
	t.Helper()

	content, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read recorded args: %v", err)
	}

	args := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	if len(args) == 1 && args[0] == "" {
		return nil
	}
	return args
}

func TestExtractWorkspaceName(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{
			name:    "workspace section",
			content: "[workspace]\nname = \"my-env\"\nchannels = [\"conda-forge\"]\n",
			want:    "my-env",
		},
		{
			name:    "project fallback",
			content: "[project]\nname = \"old-env\"\nchannels = [\"conda-forge\"]\n",
			want:    "old-env",
		},
		{
			name:    "workspace preferred over project",
			content: "[workspace]\nname = \"new\"\n[project]\nname = \"old\"\n",
			want:    "new",
		},
		{
			name:    "no name field",
			content: "[workspace]\nchannels = [\"conda-forge\"]\n",
			wantErr: true,
		},
		{
			name:    "empty content",
			content: "",
			wantErr: true,
		},
		{
			name:    "empty name field",
			content: "[workspace]\nname = \"\"\n",
			wantErr: true,
		},
		{
			name:    "name with slash rejected",
			content: "[workspace]\nname = \"data-science/fastapi\"\n",
			wantErr: true,
		},
		{
			name:    "name with colon rejected",
			content: "[workspace]\nname = \"my:env\"\n",
			wantErr: true,
		},
		{
			name:    "name with backslash rejected",
			content: "[workspace]\nname = \"my\\\\env\"\n",
			wantErr: true,
		},
		{
			name:    "dot name rejected",
			content: "[workspace]\nname = \".\"\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractWorkspaceName(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got name %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestErrorHandling tests error cases
func TestErrorHandling(t *testing.T) {
	_, err := exec.LookPath("pixi")
	if err != nil {
		t.Skip("pixi not found in PATH, skipping test")
	}

	pm, err := New()
	if err != nil {
		t.Fatalf("Failed to create PixiManager: %v", err)
	}

	ctx := context.Background()

	// Test Init with empty path
	err = pm.Init(ctx, pkgmgr.InitOptions{Name: "test"})
	if err == nil {
		t.Error("Expected error for empty EnvPath, got nil")
	}

	// Test Init with empty name
	err = pm.Init(ctx, pkgmgr.InitOptions{EnvPath: "/tmp/test"})
	if err == nil {
		t.Error("Expected error for empty Name, got nil")
	}

	// Test Install with empty path
	err = pm.Install(ctx, pkgmgr.InstallOptions{Packages: []string{"python"}})
	if err == nil {
		t.Error("Expected error for empty EnvPath, got nil")
	}

	// Test Install with no packages
	err = pm.Install(ctx, pkgmgr.InstallOptions{EnvPath: "/tmp/test"})
	if err == nil {
		t.Error("Expected error for empty Packages, got nil")
	}
}
