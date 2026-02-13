---
sidebar_position: 1
---

# Getting Started

Nebi is a local-first workspace management tool for [Pixi](https://pixi.sh). It lets you track, sync, and share reproducible environments with your team.

## Installation

### Using the install script (macOS/Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/nebari-dev/nebi/main/install.sh | bash
```

This installs the `nebi` binary to `~/.local/bin`. Make sure it's on your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### From source

```bash
go install github.com/nebari-dev/nebi/cmd/nebi@latest
```

## Quick Start

### 1. Initialize a workspace

```bash
mkdir my-project && cd my-project
nebi init
```

If no `pixi.toml` exists, `nebi init` will run `pixi init` for you automatically.

### 2. Add dependencies

```bash
pixi add python numpy pandas
```

### 3. Push to a server

```bash
nebi login https://nebi.company.com
nebi push my-project:v1.0
```

### 4. Pull on another machine

```bash
nebi login https://nebi.company.com
nebi pull my-project:v1.0
```

## What's Next?

- [CLI Overview](./cli-overview.md) — Learn about all available commands
- [CLI Workflows](./cli-workflows.md) — Step-by-step workflow examples
- [Server Setup](./server-setup.md) — Run your own Nebi server
