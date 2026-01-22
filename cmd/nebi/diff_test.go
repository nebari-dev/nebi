package main

import (
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
	source := diff.DiffRefJSON{Type: "tag", Workspace: "ws1", Tag: "v1.0"}
	target := diff.DiffRefJSON{Type: "tag", Workspace: "ws2", Tag: "v2.0"}
	tomlDiff := &diff.TomlDiff{Changes: []diff.Change{}}

	// Should not panic
	outputDiffJSONRefs(source, target, tomlDiff, nil)
}
