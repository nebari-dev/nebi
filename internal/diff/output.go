package diff

import (
	"encoding/json"
	"fmt"

	"github.com/aktech/darb/internal/drift"
)

// Exit codes for CLI commands (matches git diff and terraform plan conventions).
const (
	ExitClean = 0 // No differences
	ExitDiff  = 1 // Differences detected
	ExitError = 2 // Error occurred
)

// StatusJSON represents the JSON output format for nebi status.
type StatusJSON struct {
	Workspace    string          `json:"workspace"`
	Tag          string          `json:"tag"`
	Registry     string          `json:"registry,omitempty"`
	ServerURL    string          `json:"server_url"`
	PulledAt     string          `json:"pulled_at"`
	OriginDigest string          `json:"origin_digest"`
	Local        LocalStatusJSON `json:"local"`
	Remote       *RemoteJSON     `json:"remote,omitempty"`
}

// LocalStatusJSON represents local file status in JSON output.
type LocalStatusJSON struct {
	PixiToml string `json:"pixi_toml"`
	PixiLock string `json:"pixi_lock"`
}

// RemoteJSON represents remote status in JSON output.
type RemoteJSON struct {
	CurrentTagDigest  string `json:"current_tag_digest,omitempty"`
	TagHasMoved       bool   `json:"tag_has_moved"`
	OriginStillExists bool   `json:"origin_still_exists"`
	Error             string `json:"error,omitempty"`
}

// DiffJSON represents the JSON output format for nebi diff.
type DiffJSON struct {
	Source   DiffRefJSON  `json:"source"`
	Target   DiffRefJSON  `json:"target"`
	PixiToml *TomlDiff    `json:"pixi_toml,omitempty"`
	PixiLock *LockSummary `json:"pixi_lock,omitempty"`
}

// DiffRefJSON represents a reference in a diff (source or target).
type DiffRefJSON struct {
	Type      string `json:"type"` // "pulled", "local", "remote", "tag"
	Workspace string `json:"workspace,omitempty"`
	Tag       string `json:"tag,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Path      string `json:"path,omitempty"`
}

// LockSummary represents a summary of lock file changes.
type LockSummary struct {
	PackagesAdded   int              `json:"packages_added"`
	PackagesRemoved int              `json:"packages_removed"`
	PackagesUpdated int              `json:"packages_updated"`
	Added           []string         `json:"added,omitempty"`
	Removed         []string         `json:"removed,omitempty"`
	Updated         []PackageUpdate  `json:"updated,omitempty"`
}

// PackageUpdate represents a package version change.
type PackageUpdate struct {
	Name       string `json:"name"`
	OldVersion string `json:"old"`
	NewVersion string `json:"new"`
}

// FormatStatusJSON creates the JSON output for nebi status.
func FormatStatusJSON(ws *drift.WorkspaceStatus, workspace, tag, registry, serverURL, pulledAt, originDigest string, remote *drift.RemoteStatus) ([]byte, error) {
	status := StatusJSON{
		Workspace:    workspace,
		Tag:          tag,
		Registry:     registry,
		ServerURL:    serverURL,
		PulledAt:     pulledAt,
		OriginDigest: originDigest,
		Local: LocalStatusJSON{
			PixiToml: getFileStatusString(ws, "pixi.toml"),
			PixiLock: getFileStatusString(ws, "pixi.lock"),
		},
	}

	if remote != nil {
		status.Remote = &RemoteJSON{
			CurrentTagDigest:  remote.CurrentTagDigest,
			TagHasMoved:       remote.TagHasMoved,
			OriginStillExists: remote.Error == "",
			Error:             remote.Error,
		}
	}

	return json.MarshalIndent(status, "", "  ")
}

// FormatDiffJSON creates the JSON output for nebi diff.
func FormatDiffJSON(source, target DiffRefJSON, tomlDiff *TomlDiff, lockSummary *LockSummary) ([]byte, error) {
	diffJSON := DiffJSON{
		Source:   source,
		Target:   target,
		PixiToml: tomlDiff,
		PixiLock: lockSummary,
	}

	return json.MarshalIndent(diffJSON, "", "  ")
}

// getFileStatusString returns the status string for a file.
func getFileStatusString(ws *drift.WorkspaceStatus, filename string) string {
	fs := ws.GetFileStatus(filename)
	if fs == nil {
		return string(drift.StatusUnknown)
	}
	return string(fs.Status)
}

// ExitCodeForStatus returns the appropriate exit code for a workspace status.
func ExitCodeForStatus(ws *drift.WorkspaceStatus) int {
	if ws.IsModified() {
		return ExitDiff
	}
	return ExitClean
}

// ExitCodeForDiff returns the appropriate exit code for a diff result.
func ExitCodeForDiff(tomlDiff *TomlDiff) int {
	if tomlDiff != nil && tomlDiff.HasChanges() {
		return ExitDiff
	}
	return ExitClean
}

// FormatLockSummaryText formats a lock file summary as human-readable text.
func FormatLockSummaryText(summary *LockSummary) string {
	if summary == nil {
		return ""
	}

	total := summary.PackagesAdded + summary.PackagesRemoved + summary.PackagesUpdated
	if total == 0 {
		return "  No package changes\n"
	}

	result := fmt.Sprintf("  %d packages changed:\n", total)
	if summary.PackagesAdded > 0 {
		result += fmt.Sprintf("    Added (%d):   %s\n", summary.PackagesAdded, formatPackageList(summary.Added))
	}
	if summary.PackagesRemoved > 0 {
		result += fmt.Sprintf("    Removed (%d): %s\n", summary.PackagesRemoved, formatPackageList(summary.Removed))
	}
	if summary.PackagesUpdated > 0 {
		updates := make([]string, 0, len(summary.Updated))
		for _, u := range summary.Updated {
			updates = append(updates, fmt.Sprintf("%s (%s → %s)", u.Name, u.OldVersion, u.NewVersion))
		}
		result += fmt.Sprintf("    Updated (%d): %s\n", summary.PackagesUpdated, formatPackageList(updates))
	}

	return result
}

func formatPackageList(pkgs []string) string {
	if len(pkgs) == 0 {
		return ""
	}
	if len(pkgs) <= 5 {
		return joinPackages(pkgs)
	}
	return joinPackages(pkgs[:5]) + fmt.Sprintf(", ... (%d more)", len(pkgs)-5)
}

func joinPackages(pkgs []string) string {
	result := ""
	for i, p := range pkgs {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}
