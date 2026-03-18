# Installation

## Prerequisites

Nebi manages Pixi workspaces but **does not install Pixi for you**. Install Pixi first:

```bash
curl -fsSL https://pixi.sh/install.sh | bash
```

See the [Pixi installation docs](https://pixi.sh) for more options.

## Recommended: Install script

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

### More installation options

#### Install only CLI

```bash
curl -fsSL https://pixi.sh/install.sh | bash
```

#### Install a specific version (only CLI)

```bash
curl -fsSL https://nebi.nebari.dev/install.sh | sh -s -- --version v0.6.0-rc3
```

#### Install a specific version (with the desktop app)

```bash
curl -fsSL https://nebi.nebari.dev/install.sh | sh -s -- --version v0.6.0-rc3 --desktop
```

#### Install to a custom directory

```bash
curl -fsSL https://nebi.nebari.dev/install.sh | sh -s -- --install-dir /usr/local/bin
```

## Install from conda-forge

Nebi (CLI and desktop) package is distributed on conda-forge, you can install it with conda in your base environment:

```bash
conda install conda-forge::nebi
```

## Install from source

For certain cases like development or testing, you can install Nebi from source:

```bash
go install github.com/nebari-dev/nebi/cmd/nebi@latest
```
