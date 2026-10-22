package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist/client all:dist/server
var assets embed.FS

// ClientApp returns the browser and desktop client's built assets.
func ClientApp() (fs.FS, error) {
	return fs.Sub(assets, "dist/client")
}

// ServerApp returns the team server's built assets.
func ServerApp() (fs.FS, error) {
	return fs.Sub(assets, "dist/server")
}
