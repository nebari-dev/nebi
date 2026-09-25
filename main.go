package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// Version and Commit are set via ldflags at build time
var Version = "dev"
var Commit = ""

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create application instance
	app := NewApp()

	// Run Wails application
	err := wails.Run(&options.App{
		Title:  "Nebi - Environment Manager",
		Width:  1440,
		Height: 900,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: app.Handler(),
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
