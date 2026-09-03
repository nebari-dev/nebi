---
title: "Installation"
---

## Prerequisite

Nebi manages Pixi workspaces, install Pixi first:

```bash
curl -fsSL https://pixi.sh/install.sh | bash
```

See the [Pixi installation docs](https://pixi.sh) for more options.

## Recommended: Install Released Binaries

Install the CLI, server, and local web app from the latest GitHub release:

```bash
curl -fsSL https://nebi.nebari.dev/install.sh | sh
```

Pass `--desktop` to the installer command to install the desktop application too.

## Installation script

### Linux & MacOS

This installs the latest release to `~/.local/bin`:

```bash
curl -fsSL https://nebi.nebari.dev/install.sh | sh -s -- --desktop
```

Make sure it's on your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

**Advanced options**:

* `--version <ver>`: Install specific version (e.g. v0.5.0)
* `--install-dir <path>`: Set install directory (default: `~/.local/bin`)
* `--desktop`: Install the desktop app
* `-h`, `--help`: Show the help message

### Windows (Powershell)

```powershell
irm https://nebi.nebari.dev/install.ps1 | iex
```

**Advanced options**:

* `-Version <ver>`: Install specific version (e.g. v0.5.0)
* `-InstallDir <path>`: Set install directory (default: `$env:LOCALAPPDATA\nebi`)
* `-Desktop`: Install the desktop app

## Install with conda

The currently published conda-forge packages provide the CLI and desktop app. The CLI package installs the `nebi` command:

```bash
conda install conda-forge::nebi-cli conda-forge::nebi-desktop
```

The split `nebi-server` and `nebi-web` conda-forge packages are part of the binary split release process. Until those feedstocks are published, install those binaries from GitHub Releases or from source.

## Install from source

For certain cases like development or testing, you can install Nebi from source.

Prerequisites: Go 1.25+ and Node.js 20+

From a source checkout:

```bash
make build
```

This builds `bin/nebi`, `bin/nebi-server`, and `bin/nebi-web`.

From a source checkout, build the desktop app with Wails because it packages a native app wrapper:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
make build-desktop
```

Desktop source builds also require Node.js 20+. On Linux, install GTK/WebKit dependencies first; see the [desktop app section in the contributing guide](https://github.com/nebari-dev/nebi/blob/main/CONTRIBUTING.md#desktop-app).
