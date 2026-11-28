// Package datadir finds nebi's data directory on this machine. The CLI
// (internal/store) and the client apps (internal/client/boot) both use it,
// so they share one database and owner lock. It imports nothing from nebi,
// so internal/client can use it without depending on server packages.
package datadir

import (
	"os"
	"path/filepath"
	"runtime"
)

// EnvDataDir overrides the data directory.
const EnvDataDir = "NEBI_DATA_DIR"

// Default returns the data directory: NEBI_DATA_DIR when it is set, else
// ~/Library/Application Support/nebi on macOS, %APPDATA%\nebi on Windows
// and ~/.local/share/nebi elsewhere.
func Default() (string, error) {
	return defaultFor(runtime.GOOS, os.UserHomeDir, os.Getenv)
}

// defaultFor is Default on the operating system goos, with home and getenv
// standing in for os.UserHomeDir and os.Getenv, so every system's path can
// be tested on any.
func defaultFor(goos string, home func() (string, error), getenv func(string) string) (string, error) {
	if dir := getenv(EnvDataDir); dir != "" {
		return dir, nil
	}
	h, err := home()
	if err != nil {
		return "", err
	}
	switch goos {
	case "darwin":
		return filepath.Join(h, "Library", "Application Support", "nebi"), nil
	case "windows":
		if appData := getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "nebi"), nil
		}
		return filepath.Join(h, "AppData", "Roaming", "nebi"), nil
	default:
		return filepath.Join(h, ".local", "share", "nebi"), nil
	}
}
