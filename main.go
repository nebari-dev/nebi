// The desktop app's Wails entry point. Wails tooling needs it at the root
// of the module; all it does is call internal/client/desktop.
package main

import (
	"os"

	"github.com/nebari-dev/nebi/internal/client/api"
	"github.com/nebari-dev/nebi/internal/client/boot"
	"github.com/nebari-dev/nebi/internal/client/desktop"
	"github.com/nebari-dev/nebi/internal/frontend"
)

// Version and Commit are set via ldflags at build time
var Version = "dev"
var Commit = ""

func main() {
	// ClientApp forwards fs.Sub's filesystem and error returns. With embed.FS
	// and the fixed valid path "dist/client", this call cannot currently fail;
	// we handle the error because it remains part of ClientApp's API.
	assets, err := frontend.ClientApp()
	if err != nil {
		println("Error loading frontend:", err.Error())
		os.Exit(1)
	}
	dataDir, err := boot.DataDir()
	if err != nil {
		println("Error:", err.Error())
		os.Exit(1)
	}
	if err := desktop.Run(desktop.Options{
		DataDir:     dataDir,
		ProjectsDir: boot.ProjectsDir(),
		Version:     api.VersionInfo{NebiVersion: Version, Commit: Commit},
		Assets:      assets,
	}); err != nil {
		println("Error:", err.Error())
		os.Exit(1)
	}
}
