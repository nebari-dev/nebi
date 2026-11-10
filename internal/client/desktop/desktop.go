// Package desktop holds host capabilities only the Wails desktop app has
// (issue #649, gap 1). The UI never calls Wails bindings: it asks
// GET /api/v1/client/capabilities whether a capability exists and then uses
// the client REST API, which delegates to the implementation registered
// here. The desktop app (app.go) provides a FolderPicker; nebi-web and the
// CLI do not, so the UI falls back to a plain path field.
//
// This package is frozen; app.go owns the Wails implementation.
package desktop

import "context"

// FolderPicker opens a native "choose a folder" dialog.
type FolderPicker interface {
	// PickFolder returns the chosen absolute path, or "" when the user
	// cancelled the dialog.
	PickFolder(ctx context.Context, title, defaultPath string) (string, error)
}

// Capabilities is the set of optional host capabilities. A nil field means
// the capability is unavailable on this host.
type Capabilities struct {
	FolderPicker FolderPicker
}
