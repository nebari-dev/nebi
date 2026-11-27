package boot

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"

	"github.com/nebari-dev/nebi/internal/store"
)

// Environment variables the entry points read.
const (
	// EnvProjectsDir overrides where projects go when no path is given.
	EnvProjectsDir = "NEBI_PROJECTS_DIR"
	// EnvPixiPath names the pixi binary to use.
	EnvPixiPath = "NEBI_PIXI_PATH"
)

// DataDir returns the data directory: NEBI_DATA_DIR when it is set, and
// otherwise the operating system's directory for application data:
// ~/Library/Application Support/nebi on macOS, %APPDATA%\nebi on Windows
// and ~/.local/share/nebi elsewhere. It is the CLI's data directory
// (store.DefaultDataDir), so nebi-web, the desktop app and the CLI share
// the database and with it the owner lock.
func DataDir() (string, error) {
	dir, err := store.DefaultDataDir()
	if err != nil {
		return "", fmt.Errorf("finding the data directory: %w", err)
	}
	return dir, nil
}

// ProjectsDir returns NEBI_PROJECTS_DIR, for local.Options.ProjectsDir.
// Empty means the SDK's default, ~/nebi-projects.
func ProjectsDir() string {
	return os.Getenv(EnvProjectsDir)
}

// ErrPixiNotFound is returned by FindPixi when there is no pixi to use.
var ErrPixiNotFound = errors.New("pixi was not found")

// FindPixi returns the pixi binary to use: NEBI_PIXI_PATH when it is set,
// else pixi on PATH, else pixi where its installer puts it (~/.pixi/bin)
// or where older nebi releases did (~/.local/bin). An app started from a
// desktop launcher often has a short PATH, hence the fixed places. Nothing
// is downloaded (#630); internal/client/sdk/exec checks the version.
func FindPixi() (string, error) {
	if p := os.Getenv(EnvPixiPath); p != "" {
		return p, nil
	}
	if p, err := osexec.LookPath("pixi"); err == nil {
		return p, nil
	}
	name := "pixi"
	if runtime.GOOS == "windows" {
		name = "pixi.exe"
	}
	if home, err := os.UserHomeDir(); err == nil {
		for _, p := range []string{
			filepath.Join(home, ".pixi", "bin", name),
			filepath.Join(home, ".local", "bin", name),
		} {
			if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
				return p, nil
			} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return "", fmt.Errorf("checking %s: %w", p, err)
			}
		}
	}
	return "", fmt.Errorf("%w on PATH, in ~/.pixi/bin or in ~/.local/bin: install pixi (https://pixi.sh) or set %s to its path",
		ErrPixiNotFound, EnvPixiPath)
}
