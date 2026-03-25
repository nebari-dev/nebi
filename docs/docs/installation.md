# Installation

## Prerequisite

Nebi manages Pixi workspaces, install Pixi first:

```bash
curl -fsSL https://pixi.sh/install.sh | bash
```

See the [Pixi installation docs](https://pixi.sh) for more options.

## Recommended: Install with Pixi

Install the CLI (`nebi` package) and the desktop application (`nebi-desktop`) with [pixi global install](https://pixi.prefix.dev/latest/reference/cli/pixi/global/install/#pixi-global-install).

```bash
pixi global install nebi nebi-desktop
```

## Installation script

:::note
Supported platforms: **macOS** and **Linux**
:::

This installs the latest release of `nebi` to `~/.local/bin` (CLI and desktop app):

```bash
curl -fsSL https://nebi.nebari.dev/install.sh | sh -s --desktop
```

Make sure it's on your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### Install only CLI

```bash
curl -fsSL https://pixi.sh/install.sh | bash
```

### Install a specific version

```bash
# Only CLI
curl -fsSL https://nebi.nebari.dev/install.sh | sh -s -- --version v0.6.0-rc3

# CLI and desktop app
curl -fsSL https://nebi.nebari.dev/install.sh | sh -s -- --version v0.6.0-rc3 --desktop
```

### Install to a custom directory

```bash
curl -fsSL https://nebi.nebari.dev/install.sh | sh -s -- --install-dir /usr/local/bin
```

## Install with conda

Nebi CLI and desktop packages is distributed on conda-forge, you can install it with conda in your base environment:

```bash
conda install conda-forge::nebi
conda install conda-forge::nebi-desktop
```

## Install from source

For certain cases like development or testing, you can install Nebi from source.

:::note
Prerequisite: Go version 1.24+
:::

```bash
go install github.com/nebari-dev/nebi/cmd/nebi@latest
```
