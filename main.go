package main

import (
	"github.com/nebari-dev/nebi/internal/frontend"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
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
		return
	}

	// Create application instance
	app := NewApp()

	// Run Wails application
	err = wails.Run(&options.App{
		Title:  "Nebi - Environment Manager",
		Width:  1440,
		Height: 900,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: app.Handler(),
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
