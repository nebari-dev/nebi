---
sidebar_position: 1
---

# Getting Started

Nebi is a multi-user environment management tool for [Pixi](https://pixi.sh). It lets you track, sync, and share reproducible environments with your team.

## Prerequisites

Nebi manages Pixi workspaces but **does not install Pixi for you**. Install Pixi first:

```bash
curl -fsSL https://pixi.sh/install.sh | bash
```

See the [Pixi installation docs](https://pixi.sh) for more options.

## Installation

### Using the install script (macOS/Linux)

```bash
curl -fsSL https://nebi.nebari.dev/install.sh | sh
```

This installs the latest release of `nebi` to `~/.local/bin`. Make sure it's on your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

#### Install a specific version

```bash
curl -fsSL https://nebi.nebari.dev/install.sh | sh -s -- --version v0.6.0-rc3
```

#### Install with the desktop app

```bash
curl -fsSL https://nebi.nebari.dev/install.sh | sh -s -- --version v0.6.0-rc3 --desktop
```

#### Install to a custom directory

```bash
curl -fsSL https://nebi.nebari.dev/install.sh | sh -s -- --install-dir /usr/local/bin
```

### From source

```bash
go install github.com/nebari-dev/nebi/cmd/nebi@latest
```

## What's Next?

- [CLI Guide](./cli-guide.md) — Local workflows and team collaboration
- [CLI Reference](./cli-reference.md) — All available commands at a glance
- [Server Setup](./server-setup.md) — Run your own Nebi server
