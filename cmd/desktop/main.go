package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/openteams-ai/darb/desktop"
	"github.com/openteams-ai/darb/internal/api/handlers"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// Version is set via ldflags at build time
var Version = "dev"

func main() {
	// Set version in handlers and desktop package
	handlers.Version = Version
	desktop.Version = Version

	// Create application instance
	app := desktop.NewApp()

	// Custom startup that waits for server to be ready
	onStartup := func(ctx context.Context) {
		// Start the embedded server
		app.Startup(ctx)

		// Wait for the server to be ready (with timeout)
		timeout := time.After(30 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				slog.Error("Timeout waiting for server to start")
				return
			case <-ticker.C:
				if app.IsReady() {
					slog.Info("Server is ready, loading UI")
					return
				}
			}
		}
	}

	// Create application with options
	// Uses AssetServer with Handler to proxy to embedded HTTP server
	err := wails.Run(&options.App{
		Title:     "Darb",
		Width:     1280,
		Height:    800,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Handler: desktop.NewProxyHandler("http://127.0.0.1:8460"),
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		OnStartup:        onStartup,
		OnShutdown:       app.Shutdown,
		Bind: []interface{}{
			app,
		},
		// macOS specific options
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: false,
				HideTitle:                  false,
				HideTitleBar:               false,
				FullSizeContent:            false,
				UseToolbar:                 false,
			},
			About: &mac.AboutInfo{
				Title:   "Darb Desktop",
				Message: "Multi-User Environment Management System\n\nVersion: " + Version,
			},
		},
		// Windows specific options
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
		// Linux specific options
		Linux: &linux.Options{
			WindowIsTranslucent: false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
