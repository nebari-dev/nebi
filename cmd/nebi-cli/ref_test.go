package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nebari-dev/nebi/internal/store"
)

func TestIsPath(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		// Paths (contain a slash)
		{"./foo", true},
		{"../bar", true},
		{"/tmp/project", true},
		{"foo/bar", true},
		{".", true},  // current directory
		{"..", true}, // parent directory

		// Names (no slash)
		{"data-science", false},
		{"myproject", false},
		{"my_env", false},

		// Server refs (colon but no slash)
		{"myproject:v1", false},
		{"env:latest", false},

		// Windows-style paths (backslash = filepath.Separator on Windows)
		{`foo` + string(filepath.Separator) + `bar`, true},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got := isPath(tt.ref)
			if got != tt.want {
				t.Errorf("isPath(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestValidateProjectName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		// Valid names
		{"data-science", false},
		{"my_env", false},
		{"env123", false},
		{"ML-Project", false},

		// Invalid: contains path separators
		{"foo/bar", true},
		{"./foo", true},
		{`foo\bar`, true},

		// Invalid: contains colon (ambiguous with server refs)
		{"env:v1", true},

		// Invalid: empty
		{"", true},

		// Invalid: reserved names (path-like)
		{".", true},
		{"..", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProjectName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProjectName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestSyncProjectNameRejectsInvalidName(t *testing.T) {
	// Create a temp dir with a pixi.toml that has an invalid name (contains slash)
	tmpDir := t.TempDir()
	pixiToml := `[workspace]
name = "data-science/fastapi"
channels = ["conda-forge"]
platforms = ["linux-64"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "pixi.toml"), []byte(pixiToml), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &store.LocalProject{
		Name: "data-science",
		Path: tmpDir,
	}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	// syncProjectName should return an error for the invalid name
	err = syncProjectName(s, project)
	if err == nil {
		t.Fatal("expected error for invalid project name with slash, got nil")
	}

	// Verify the stored name was NOT updated
	got, err := s.GetProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "data-science" {
		t.Errorf("stored name should not have changed, got %q", got.Name)
	}
}

func TestSyncProjectNameUpdatesValidName(t *testing.T) {
	// Create a temp dir with a pixi.toml that has a different but valid name
	tmpDir := t.TempDir()
	pixiToml := `[workspace]
name = "ml-project"
channels = ["conda-forge"]
platforms = ["linux-64"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "pixi.toml"), []byte(pixiToml), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &store.LocalProject{
		Name: "old-name",
		Path: tmpDir,
	}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	// syncProjectName should succeed and update the name
	err = syncProjectName(s, project)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := s.GetProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "ml-project" {
		t.Errorf("expected name %q, got %q", "ml-project", got.Name)
	}
}
