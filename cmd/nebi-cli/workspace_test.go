package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceDirMissing(t *testing.T) {
	tests := []struct {
		name  string
		files []string // files to create inside the workspace dir
		noDir bool     // do not create the directory at all
		want  bool
	}{
		{name: "directory does not exist", noDir: true, want: true},
		{name: "empty directory", want: true},
		{name: "unrelated files only", files: []string{"README.md"}, want: true},
		{name: "manifest only", files: []string{"pixi.toml"}, want: false},
		{name: "lockfile only", files: []string{"pixi.lock"}, want: false},
		{name: "manifest and lockfile", files: []string{"pixi.toml", "pixi.lock"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "ws")
			if !tt.noDir {
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				for _, f := range tt.files {
					if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			if got := workspaceDirMissing(dir); got != tt.want {
				t.Errorf("workspaceDirMissing(%q) = %v, want %v", dir, got, tt.want)
			}
		})
	}
}
