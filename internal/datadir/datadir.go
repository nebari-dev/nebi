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
	if dir := os.Getenv(EnvDataDir); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "nebi"), nil
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "nebi"), nil
		}
		return filepath.Join(home, "AppData", "Roaming", "nebi"), nil
	default:
		return filepath.Join(home, ".local", "share", "nebi"), nil
	}
}
