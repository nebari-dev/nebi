package oci

import (
	"strings"
	"testing"
)

func TestValidateAssetPath(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		wantErr string // substring; empty = expect nil
	}{
		// Valid
		{"simple file", "README.md", ""},
		{"nested", "src/main.go", ""},
		{"deep nested", "a/b/c/d/e.txt", ""},
		{"subdir pixi toml", "subdir/pixi.toml", ""},
		{"dot file", ".env.sample", ""},
		{"unicode ok", "données.txt", ""},

		// Absolute & separator
		{"absolute posix", "/etc/passwd", "absolute"},
		{"absolute win drive", "C:\\foo", "absolute"},
		{"absolute backslash", "\\foo", "absolute"},
		{"backslash sep", "foo\\bar", "backslash"},

		// Traversal & canonical
		{"dotdot root", "../escape", "parent"},
		{"dotdot inner", "src/../etc", "non-canonical"},
		{"dot segment", "./foo", "non-canonical"},
		{"double slash", "src//main.go", "non-canonical"},
		{"trailing slash", "src/", "non-canonical"},

		// Control chars
		{"null byte", "foo\x00bar", "control"},
		{"tab", "foo\tbar", "control"},
		{"newline", "foo\nbar", "control"},
		{"del", "foo\x7f", "control"},

		// Windows-hostile
		{"reserved con", "CON", "reserved"},
		{"reserved con ext", "con.txt", "reserved"},
		{"reserved lpt9", "LPT9.log", "reserved"},
		{"reserved com1 nested", "logs/COM1.txt", "reserved"},
		{"trailing dot", "file.", "dot or space"},
		{"trailing space", "file ", "dot or space"},
		{"trailing dot nested", "foo/bar.", "dot or space"},

		// Reserved root
		{"pixi toml root", "pixi.toml", "core layer"},
		{"pixi lock root", "pixi.lock", "core layer"},
		// Case-insensitive variants of core names: collide on
		// case-insensitive filesystems (Windows, default macOS) with the
		// core pixi.toml / pixi.lock written at extract time.
		{"pixi toml case", "Pixi.toml", "core layer"},
		{"pixi lock case", "PIXI.LOCK", "core layer"},

		// Empty
		{"empty", "", "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAssetPath(tc.path)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidateAssetPaths_Collision(t *testing.T) {
	cases := []struct {
		name    string
		paths   []string
		wantErr string
	}{
		{"exact dup", []string{"a.txt", "a.txt"}, "collision"},
		{"case dup", []string{"README.md", "readme.md"}, "collision"},
		{"nfc dup", []string{"café.txt", "cafe\u0301.txt"}, "collision"},
		{"unique ok", []string{"a.txt", "b.txt", "sub/a.txt"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAssetPaths(tc.paths)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateAssetPaths_FirstFailWins(t *testing.T) {
	err := validateAssetPaths([]string{"ok.txt", "../bad", "also_ok.txt"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parent") {
		t.Fatalf("expected parent-segment error, got %v", err)
	}
}
