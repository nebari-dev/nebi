---
title: "Installation"
---

## Prerequisite

Nebi manages Pixi workspaces, install Pixi first:

```bash
curl -fsSL https://pixi.sh/install.sh | bash
```

See the [Pixi installation docs](https://pixi.sh) for more options.

## Recommended: Install with Pixi

Install the recommended `nebi` package with [pixi global install](https://pixi.prefix.dev/latest/reference/cli/pixi/global/install/#pixi-global-install). The package installs the `nebi` CLI command and the desktop app:

```bash
pixi global install nebi
```

If you only need one part, install the underlying package directly. The CLI-only package is named `nebi-cli`, but the command it installs is still `nebi`:

```bash
pixi global install nebi-cli
pixi global install nebi-desktop
```

## Installation script

### Linux & MacOS

This installs the latest `nebi`, `nebi-server`, and `nebi-web` release binaries to `~/.local/bin`, plus the desktop app:

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

The same packages are distributed on conda-forge. The `nebi` package installs the `nebi` CLI command and the desktop app:

```bash
conda install conda-forge::nebi
```

## Install from source

For certain cases like development or testing, you can install Nebi from source.

Prerequisites: Go 1.25+ and Node.js 20+

From a source checkout:

```bash
make build
```

This builds `bin/nebi`, `bin/nebi-server`, and `bin/nebi-web`.

### Build Docker images locally

The Dockerfile uses explicit targets for each image. Pass `--target` to choose which binary goes into the final image:

```bash
# Team server image
docker build --target nebi-server -t nebi:local .

# Local web image
docker build --target nebi-web -t nebi-web:local .
```

The image tag (`-t`) only names the image. It does not choose the Dockerfile target.

From a source checkout, build the desktop app with Wails because it packages a native app wrapper:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
make build-desktop
```

Desktop source builds also require Node.js 20+. On Linux, install GTK/WebKit dependencies first; see the [desktop app section in the contributing guide](https://github.com/nebari-dev/nebi/blob/main/CONTRIBUTING.md#desktop-app).
