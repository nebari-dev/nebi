package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aktech/darb/internal/diff"
)

func TestDiffCmd_HasFlags(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		defValue string
	}{
		{"remote", "remote", "false"},
		{"json", "json", "false"},
		{"lock", "lock", "false"},
		{"toml", "toml", "false"},
		{"path", "path", "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := diffCmd.Flags().Lookup(tt.flag)
			if flag == nil {
				t.Fatalf("--%s flag should be registered", tt.flag)
			}
			if flag.DefValue != tt.defValue {
				t.Errorf("--%s default = %q, want %q", tt.flag, flag.DefValue, tt.defValue)
			}
		})
	}
}

func TestDiffCmd_AcceptsUpToTwoArgs(t *testing.T) {
	if diffCmd.Args == nil {
		t.Fatal("Args should not be nil")
	}
}

func TestOutputDiffText_NoChanges(t *testing.T) {
	tomlDiff := &diff.TomlDiff{Changes: []diff.Change{}}
	lockContent := []byte("same")

	// Should not panic with no changes
	outputDiffText(tomlDiff, nil, lockContent, lockContent, "source", "target")
}

func TestOutputDiffText_TomlChangesOnly(t *testing.T) {
	tomlDiff := &diff.TomlDiff{
		Changes: []diff.Change{
			{Section: "dependencies", Key: "numpy", Type: diff.ChangeAdded, NewValue: ">=1.0"},
		},
	}
	lockContent := []byte("same")

	// Should not panic
	outputDiffText(tomlDiff, nil, lockContent, lockContent, "source", "target")
}

func TestOutputDiffText_LockChangesWithSummary(t *testing.T) {
	tomlDiff := &diff.TomlDiff{Changes: []diff.Change{}}
	oldLock := []byte("old lock content")
	newLock := []byte("new lock content")
	summary := &diff.LockSummary{
		PackagesAdded: 2,
		PackagesUpdated: 1,
		Added: []string{"scipy 1.11", "torch 2.0"},
		Updated: []diff.PackageUpdate{
			{Name: "numpy", OldVersion: "1.0", NewVersion: "2.0"},
		},
	}

	// Should not panic with lock changes
	outputDiffText(tomlDiff, summary, oldLock, newLock, "source", "target")
}

func TestOutputDiffText_LockFlag(t *testing.T) {
	// Test with --lock flag set
	origDiffLock := diffLock
	diffLock = true
	defer func() { diffLock = origDiffLock }()

	tomlDiff := &diff.TomlDiff{Changes: []diff.Change{}}
	oldLock := []byte("old")
	newLock := []byte("new")
	summary := &diff.LockSummary{
		PackagesAdded: 1,
		Added:         []string{"scipy 1.11"},
	}

	// Should show full lock diff with --lock
	outputDiffText(tomlDiff, summary, oldLock, newLock, "source", "target")
}

func TestOutputDiffText_TomlOnly(t *testing.T) {
	origDiffToml := diffToml
	diffToml = true
	defer func() { diffToml = origDiffToml }()

	tomlDiff := &diff.TomlDiff{
		Changes: []diff.Change{
			{Section: "dependencies", Key: "numpy", Type: diff.ChangeAdded, NewValue: ">=1.0"},
		},
	}

	// Should only show TOML diff, no lock info
	outputDiffText(tomlDiff, nil, []byte("a"), []byte("b"), "source", "target")
}

func TestTruncateDigest(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"sha256:abcdef1234567890abcdef", "sha256:abcdef123456..."},
		{"sha256:short", "sha256:short"},
		{"", ""},
	}

	for _, tt := range tests {
		result := truncateDigest(tt.input)
		if result != tt.expected {
			t.Errorf("truncateDigest(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestBytesEqual(t *testing.T) {
	tests := []struct {
		a, b     []byte
		expected bool
	}{
		{[]byte("hello"), []byte("hello"), true},
		{[]byte("hello"), []byte("world"), false},
		{[]byte("hello"), []byte("hell"), false},
		{[]byte{}, []byte{}, true},
		{nil, nil, true},
		{nil, []byte{}, true},
	}

	for _, tt := range tests {
		result := bytesEqual(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("bytesEqual(%q, %q) = %v, want %v", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestOutputDiffJSONRefs(t *testing.T) {
	source := diff.DiffRefJSON{Type: "tag", Repo: "ws1", Tag: "v1.0"}
	target := diff.DiffRefJSON{Type: "tag", Repo: "ws2", Tag: "v2.0"}
	tomlDiff := &diff.TomlDiff{Changes: []diff.Change{}}

	// Should not panic
	outputDiffJSONRefs(source, target, tomlDiff, nil)
}

func TestIsPathLike(t *testing.T) {
	tests := []struct {
		arg  string
		want bool
	}{
		{".", true},
		{"./foo", true},
		{"../bar", true},
		{"/absolute/path", true},
		{"~/projects/foo", true},
		{"~user/foo", true},
		{"data-science:v1.0", false},
		{"workspace", false},
		{"my-ws:latest", false},
		{"foo/bar", false}, // relative without ./ prefix is NOT path-like
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			got := isPathLike(tt.arg)
			if got != tt.want {
				t.Errorf("isPathLike(%q) = %v, want %v", tt.arg, got, tt.want)
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	t.Run("absolute path unchanged", func(t *testing.T) {
		got, err := resolvePath("/tmp/foo")
		if err != nil {
			t.Fatal(err)
		}
		if got != "/tmp/foo" {
			t.Errorf("resolvePath('/tmp/foo') = %q, want '/tmp/foo'", got)
		}
	})

	t.Run("dot resolves to cwd", func(t *testing.T) {
		cwd, _ := os.Getwd()
		got, err := resolvePath(".")
		if err != nil {
			t.Fatal(err)
		}
		if got != cwd {
			t.Errorf("resolvePath('.') = %q, want %q", got, cwd)
		}
	})

	t.Run("tilde expands to home", func(t *testing.T) {
		home, _ := os.UserHomeDir()
		got, err := resolvePath("~/foo")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, "foo")
		if got != want {
			t.Errorf("resolvePath('~/foo') = %q, want %q", got, want)
		}
	})
}

func TestReadLocalWorkspace(t *testing.T) {
	t.Run("reads pixi.toml and pixi.lock", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("[workspace]\nname = \"test\""), 0644)
		os.WriteFile(filepath.Join(dir, "pixi.lock"), []byte("version: 6"), 0644)

		toml, lock, err := readLocalWorkspace(dir)
		if err != nil {
			t.Fatal(err)
		}
		if string(toml) != "[workspace]\nname = \"test\"" {
			t.Errorf("unexpected toml: %q", toml)
		}
		if string(lock) != "version: 6" {
			t.Errorf("unexpected lock: %q", lock)
		}
	})

	t.Run("pixi.lock optional", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("[workspace]"), 0644)

		toml, lock, err := readLocalWorkspace(dir)
		if err != nil {
			t.Fatal(err)
		}
		if string(toml) != "[workspace]" {
			t.Errorf("unexpected toml: %q", toml)
		}
		if lock != nil {
			t.Errorf("expected nil lock, got %q", lock)
		}
	})

	t.Run("error when pixi.toml missing", func(t *testing.T) {
		dir := t.TempDir()
		_, _, err := readLocalWorkspace(dir)
		if err == nil {
			t.Fatal("expected error for missing pixi.toml")
		}
	})
}

func TestRunDiffTwoPaths(t *testing.T) {
	// Create two temp workspace directories with different content
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	toml1 := `[workspace]
name = "test"

[dependencies]
numpy = ">=1.0"
`
	toml2 := `[workspace]
name = "test"

[dependencies]
numpy = ">=2.0"
scipy = ">=1.0"
`
	os.WriteFile(filepath.Join(dir1, "pixi.toml"), []byte(toml1), 0644)
	os.WriteFile(filepath.Join(dir2, "pixi.toml"), []byte(toml2), 0644)

	// readLocalWorkspace should work for both
	t1, _, err := readLocalWorkspace(dir1)
	if err != nil {
		t.Fatal(err)
	}
	t2, _, err := readLocalWorkspace(dir2)
	if err != nil {
		t.Fatal(err)
	}

	// The diff engine should detect changes
	tomlDiff, err := diff.CompareToml(t1, t2)
	if err != nil {
		t.Fatal(err)
	}
	if !tomlDiff.HasChanges() {
		t.Error("expected changes between the two workspaces")
	}
}
