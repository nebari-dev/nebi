package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aktech/darb/internal/drift"
	"github.com/aktech/darb/internal/nebifile"
)

// formatTimeAgo formats a time as a human-readable "X ago" string.
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		months := int(d.Hours() / 24 / 30)
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	}
}

// formatStatusJSONInternal produces JSON output for nebi status.
func formatStatusJSONInternal(ws *drift.WorkspaceStatus, nf *nebifile.NebiFile, remote *drift.RemoteStatus) ([]byte, error) {
	type localStatus struct {
		PixiToml string `json:"pixi_toml"`
		PixiLock string `json:"pixi_lock"`
	}

	type remoteJSON struct {
		CurrentTagDigest  string `json:"current_tag_digest,omitempty"`
		TagHasMoved       bool   `json:"tag_has_moved"`
		OriginStillExists bool   `json:"origin_still_exists"`
		Error             string `json:"error,omitempty"`
	}

	type statusOutput struct {
		Workspace    string      `json:"workspace"`
		Tag          string      `json:"tag"`
		RegistryURL  string      `json:"registry_url,omitempty"`
		ServerURL    string      `json:"server_url"`
		PulledAt     string      `json:"pulled_at"`
		OriginDigest string      `json:"origin_digest"`
		Local        localStatus `json:"local"`
		Remote       *remoteJSON `json:"remote,omitempty"`
	}

	output := statusOutput{
		Workspace:    nf.Origin.Workspace,
		Tag:          nf.Origin.Tag,
		RegistryURL:  nf.Origin.RegistryURL,
		ServerURL:    nf.Origin.ServerURL,
		PulledAt:     nf.Origin.PulledAt.Format(time.RFC3339),
		OriginDigest: nf.Origin.ManifestDigest,
		Local: localStatus{
			PixiToml: getFileStatus(ws, "pixi.toml"),
			PixiLock: getFileStatus(ws, "pixi.lock"),
		},
	}

	if remote != nil {
		output.Remote = &remoteJSON{
			CurrentTagDigest:  remote.CurrentTagDigest,
			TagHasMoved:       remote.TagHasMoved,
			OriginStillExists: remote.Error == "",
			Error:             remote.Error,
		}
	}

	return json.MarshalIndent(output, "", "  ")
}

func getFileStatus(ws *drift.WorkspaceStatus, filename string) string {
	fs := ws.GetFileStatus(filename)
	if fs == nil {
		return string(drift.StatusUnknown)
	}
	return string(fs.Status)
}
