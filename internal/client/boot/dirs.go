package boot

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nebari-dev/nebi/internal/datadir"
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
// (datadir.Default, which store.DefaultDataDir uses too), so nebi-web, the
// desktop app and the CLI share the database and with it the owner lock.
func DataDir() (string, error) {
	dir, err := datadir.Default()
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
// else pixi on PATH, else the first pixi in pixiDirs. Nothing is downloaded
// (#630); internal/client/sdk/exec checks the version.
func FindPixi() (string, error) {
	if p := os.Getenv(EnvPixiPath); p != "" {
		return p, nil
	}
	if p, err := osexec.LookPath("pixi"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	return findPixiIn(pixiDirs(home, runtime.GOOS))
}

// findPixiIn returns the first pixi in dirs.
func findPixiIn(dirs []string) (string, error) {
	name := "pixi"
	if runtime.GOOS == "windows" {
		name = "pixi.exe"
	}
	for _, dir := range dirs {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p, nil
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("checking %s: %w", p, err)
		}
	}
	return "", fmt.Errorf("%w on PATH or in %s: install pixi (https://pixi.sh) or set %s to its path",
		ErrPixiNotFound, strings.Join(dirs, ", "), EnvPixiPath)
}

// homebrewBins are the folders Homebrew installs commands into: on Apple
// silicon, then on Intel. Tests replace them.
var homebrewBins = []string{"/opt/homebrew/bin", "/usr/local/bin"}

// pixiDirs are the folders FindPixi looks in when pixi is not on PATH, in
// order. An app started from a desktop launcher often has a short PATH: on
// macOS, an app started from Finder or the Dock gets only
// /usr/bin:/bin:/usr/sbin:/sbin and none of the shell's variables. So it
// looks where pixi's installer puts it (~/.pixi/bin), then, on macOS, where
// Homebrew does, and last where older nebi releases put it (~/.local/bin),
// which can hold a pixi too old to use.
func pixiDirs(home, goos string) []string {
	var dirs []string
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".pixi", "bin"))
	}
	if goos == "darwin" {
		dirs = append(dirs, homebrewBins...)
	}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	return dirs
}
