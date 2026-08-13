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

Install the CLI (`nebi` package) and the desktop application (`nebi-desktop`) with [pixi global install](https://pixi.prefix.dev/latest/reference/cli/pixi/global/install/#pixi-global-install).

```bash
pixi global install nebi
```

## Installation script

### Linux & MacOS

This installs the latest release of `nebi` to `~/.local/bin` (CLI and desktop app):

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

Nebi CLI and desktop packages are distributed on conda-forge, you can install it with conda in your base environment:

```bash
conda install conda-forge::nebi
```

## Install from source

For certain cases like development or testing, you can install Nebi from source.

Prerequisite: Go version 1.24+

```bash
go install github.com/nebari-dev/nebi/cmd/nebi@latest
```
