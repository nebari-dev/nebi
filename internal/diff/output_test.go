package diff

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aktech/darb/internal/drift"
)

func TestExitCodeForStatus_Clean(t *testing.T) {
	ws := &drift.RepoStatus{Overall: drift.StatusClean}
	code := ExitCodeForStatus(ws)
	if code != ExitClean {
		t.Errorf("ExitCodeForStatus(clean) = %d, want %d", code, ExitClean)
	}
}

func TestExitCodeForStatus_Modified(t *testing.T) {
	ws := &drift.RepoStatus{
		Overall: drift.StatusModified,
		Files: []drift.FileStatus{
			{Filename: "pixi.toml", Status: drift.StatusModified},
		},
	}
	code := ExitCodeForStatus(ws)
	if code != ExitDiff {
		t.Errorf("ExitCodeForStatus(modified) = %d, want %d", code, ExitDiff)
	}
}

func TestExitCodeForDiff_NoChanges(t *testing.T) {
	d := &TomlDiff{Changes: []Change{}}
	code := ExitCodeForDiff(d)
	if code != ExitClean {
		t.Errorf("ExitCodeForDiff(no changes) = %d, want %d", code, ExitClean)
	}
}

func TestExitCodeForDiff_HasChanges(t *testing.T) {
	d := &TomlDiff{Changes: []Change{{Type: ChangeAdded}}}
	code := ExitCodeForDiff(d)
	if code != ExitDiff {
		t.Errorf("ExitCodeForDiff(has changes) = %d, want %d", code, ExitDiff)
	}
}

func TestExitCodeForDiff_Nil(t *testing.T) {
	code := ExitCodeForDiff(nil)
	if code != ExitClean {
		t.Errorf("ExitCodeForDiff(nil) = %d, want %d", code, ExitClean)
	}
}

func TestFormatStatusJSON(t *testing.T) {
	ws := &drift.RepoStatus{
		Overall: drift.StatusModified,
		Files: []drift.FileStatus{
			{Filename: "pixi.toml", Status: drift.StatusModified},
			{Filename: "pixi.lock", Status: drift.StatusClean},
		},
	}

	remote := &drift.RemoteStatus{
		OriginDigest:     "sha256:aaa",
		CurrentTagDigest: "sha256:bbb",
		TagHasMoved:      true,
	}

	data, err := FormatStatusJSON(ws, "data-science", "v1.0", "ds-team",
		"https://nebi.example.com", "2025-01-15T10:30:00Z", "sha256:aaa", remote)
	if err != nil {
		t.Fatalf("FormatStatusJSON() error = %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if result["workspace"] != "data-science" {
		t.Errorf("workspace = %v, want %q", result["workspace"], "data-science")
	}
	if result["tag"] != "v1.0" {
		t.Errorf("tag = %v, want %q", result["tag"], "v1.0")
	}

	local := result["local"].(map[string]interface{})
	if local["pixi_toml"] != "modified" {
		t.Errorf("pixi_toml = %v, want %q", local["pixi_toml"], "modified")
	}
	if local["pixi_lock"] != "clean" {
		t.Errorf("pixi_lock = %v, want %q", local["pixi_lock"], "clean")
	}

	remoteResult := result["remote"].(map[string]interface{})
	if remoteResult["tag_has_moved"] != true {
		t.Errorf("tag_has_moved = %v, want true", remoteResult["tag_has_moved"])
	}
}

func TestFormatStatusJSON_NoRemote(t *testing.T) {
	ws := &drift.RepoStatus{
		Overall: drift.StatusClean,
		Files: []drift.FileStatus{
			{Filename: "pixi.toml", Status: drift.StatusClean},
			{Filename: "pixi.lock", Status: drift.StatusClean},
		},
	}

	data, err := FormatStatusJSON(ws, "test", "v1.0", "", "https://example.com",
		"2025-01-15T10:30:00Z", "sha256:abc", nil)
	if err != nil {
		t.Fatalf("FormatStatusJSON() error = %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	if result["remote"] != nil {
		t.Error("remote should be nil when not provided")
	}
}

func TestFormatDiffJSON(t *testing.T) {
	source := DiffRefJSON{
		Type:      "pulled",
		Repo: "data-science",
		Tag:       "v1.0",
		Digest:    "sha256:abc",
	}
	target := DiffRefJSON{
		Type: "local",
		Path: "/home/user/project",
	}

	tomlDiff := &TomlDiff{
		Changes: []Change{
			{Section: "dependencies", Key: "scipy", Type: ChangeAdded, NewValue: ">=1.17"},
		},
	}

	lockSummary := &LockSummary{
		PackagesAdded:   3,
		PackagesRemoved: 1,
		PackagesUpdated: 2,
	}

	data, err := FormatDiffJSON(source, target, tomlDiff, lockSummary)
	if err != nil {
		t.Fatalf("FormatDiffJSON() error = %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	sourceResult := result["source"].(map[string]interface{})
	if sourceResult["type"] != "pulled" {
		t.Errorf("source.type = %v, want %q", sourceResult["type"], "pulled")
	}

	targetResult := result["target"].(map[string]interface{})
	if targetResult["type"] != "local" {
		t.Errorf("target.type = %v, want %q", targetResult["type"], "local")
	}
}

func TestFormatLockSummaryText_NoChanges(t *testing.T) {
	summary := &LockSummary{}
	result := FormatLockSummaryText(summary)
	if !strings.Contains(result, "No package changes") {
		t.Errorf("Should indicate no changes, got %q", result)
	}
}

func TestFormatLockSummaryText_WithChanges(t *testing.T) {
	summary := &LockSummary{
		PackagesAdded:   2,
		PackagesRemoved: 1,
		PackagesUpdated: 3,
		Added:           []string{"scipy 1.17", "torch 2.0"},
		Removed:         []string{"old-pkg 1.0"},
		Updated: []PackageUpdate{
			{Name: "numpy", OldVersion: "2.0", NewVersion: "2.4"},
			{Name: "python", OldVersion: "3.11", NewVersion: "3.12"},
			{Name: "pip", OldVersion: "23.0", NewVersion: "24.0"},
		},
	}

	result := FormatLockSummaryText(summary)

	if !strings.Contains(result, "6 packages changed") {
		t.Errorf("Should show total count, got %q", result)
	}
	if !strings.Contains(result, "Added (2)") {
		t.Error("Should show added count")
	}
	if !strings.Contains(result, "Removed (1)") {
		t.Error("Should show removed count")
	}
	if !strings.Contains(result, "Updated (3)") {
		t.Error("Should show updated count")
	}
}

func TestFormatLockSummaryText_Nil(t *testing.T) {
	result := FormatLockSummaryText(nil)
	if result != "" {
		t.Errorf("Should be empty for nil, got %q", result)
	}
}

func TestFormatLockSummaryText_ManyPackages(t *testing.T) {
	summary := &LockSummary{
		PackagesAdded: 7,
		Added:         []string{"a", "b", "c", "d", "e", "f", "g"},
	}

	result := FormatLockSummaryText(summary)
	if !strings.Contains(result, "... (2 more)") {
		t.Errorf("Should truncate long package lists, got %q", result)
	}
}

func TestExitCodes(t *testing.T) {
	if ExitClean != 0 {
		t.Errorf("ExitClean = %d, want 0", ExitClean)
	}
	if ExitDiff != 1 {
		t.Errorf("ExitDiff = %d, want 1", ExitDiff)
	}
	if ExitError != 2 {
		t.Errorf("ExitError = %d, want 2", ExitError)
	}
}
