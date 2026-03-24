# Nebi

<div align="center">
  <img src="assets/nebi-icon-solid.svg" alt="Nebi" width="200"/>
</div>

<p align="center">
  Multi-user environment management for Pixi
</p>

<p align="center">
  <a href="https://github.com/nebari-dev/nebi/actions/workflows/ci.yml">
    <img src="https://github.com/nebari-dev/nebi/actions/workflows/ci.yml/badge.svg" alt="CI">
  </a>
  <a href="https://github.com/nebari-dev/nebi/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/nebari-dev/nebi" alt="License">
  </a>
  <a href="https://github.com/nebari-dev/nebi/releases">
    <img src="https://img.shields.io/github/v/release/nebari-dev/nebi?include_prereleases" alt="Release">
  </a>
  <a href="https://github.com/nebari-dev/nebi/issues">
    <img src="https://img.shields.io/github/issues/nebari-dev/nebi" alt="Issues">
  </a>
  <a href="https://github.com/nebari-dev/nebi/pulls">
    <img src="https://img.shields.io/github/issues-pr/nebari-dev/nebi" alt="Pull Requests">
  </a>
</p>

---

> **⚠️ Alpha Software**: Nebi is currently in alpha. APIs, UI, CLI and available features may change without notice. Not recommended for production use.

![Nebi Demo](https://raw.githubusercontent.com/nebari-dev/nebi-video-demo-automation/25e0139cf70cc0e9f8a2cf938fddd85ecd83adee/assets/demo.gif)

![Nebi CLI Demo](assets/nebi-demo.gif)

## What is Nebi?

If your Python projects need compiled libraries like GDAL or CUDA, you know `pip install` often isn't enough. [Pixi](https://pixi.sh) solves that by managing both Python packages and system libraries in one lockfile.

Nebi builds on Pixi to add what teams need: version history, rollback, sharing environments through registries, and access control over who can change production dependencies.

**Key features:**

- Install system libraries alongside Python packages (via Pixi)
- Push, pull, and diff versioned environments across machines
- Share environments through OCI registries (Quay.io, GHCR, etc.)
- Roll back when a dependency update breaks your workflow
- Control who can modify shared environments with role-based access
- Activate any workspace by name from any directory

## Quick Start

### Quick Install

**Linux / macOS:**
```sh
curl -fsSL https://nebi.nebari.dev/install.sh | sh
```

**Windows (PowerShell):**
```powershell
irm https://nebi.nebari.dev/install.ps1 | iex
```

See `install.sh --help` or the script source for advanced options (`--version`, `--install-dir`, `--desktop`).

### CLI Quick Start

#### Set up

Start a local Nebi server (set your own admin credentials):

```bash
export ADMIN_USERNAME=admin
export ADMIN_PASSWORD=your-password
nebi serve
```

This starts the Nebi server at [http://localhost:8460](http://localhost:8460).

In a new terminal, connect the CLI to the server:

```bash
nebi login http://localhost:8460
```

#### Track and push your environment

Initialize nebi in your project:

```bash
cd my-project
nebi init
```

Push your environment to the server:

```bash
nebi push myworkspace
```

Or tag a specific version:

```bash
nebi push myworkspace:v1.0
```

Once pushed, your workspace appears on the server dashboard:

![Workspace](assets/workspaces.png)

#### Pull on another machine

Pull an environment on another machine:

```bash
nebi pull myworkspace:v1.0
```

Verify what version you're running with `nebi status`:

```bash
nebi status
```

```text
Workspace: myworkspace
Path:      /Users/you/my-project
Server:    http://localhost:8460
Origin:    myworkspace:v1.0 (pull)
```

## CLI Usage

### Initialize and track

Track a pixi workspace (runs `pixi init` if no `pixi.toml` exists):

```bash
cd my-project
nebi init
```

Check sync status:

```bash
nebi status
```

List all tracked workspaces:

```bash
nebi workspace list
```

### Push and pull

Push your environment to the server:

```bash
nebi push myworkspace                    # auto-tags: sha-<hash> + latest
nebi push myworkspace:v1.0               # also adds user tag v1.0
nebi push                                # reuse workspace name from origin
```

Pull an environment:

```bash
nebi pull myworkspace:v1.0
nebi pull                                # re-pull from last origin
```

List workspaces and tags on a server:

```bash
nebi workspace list --remote
nebi workspace tags myworkspace
```

### Compare environments

Compare against the last pushed or pulled version:

```bash
nebi diff
```

Compare between directories:

```bash
nebi diff ./project-a ./project-b
nebi diff ./project-a ./project-b --lock    # also compare pixi.lock
```

Compare server versions or workspace names:

```bash
nebi diff myworkspace:v1 myworkspace:v2
nebi diff data-science ./my-project
nebi diff data-science ml-pipeline
```

### Run tasks and shells

Open a shell in any workspace by name:

```bash
nebi shell data-science
nebi shell data-science -e dev          # args pass through to pixi shell
```

Run pixi tasks:

```bash
nebi run my-task                        # run a pixi task (auto-initializes workspace)
nebi run data-science my-task           # run a task in a named workspace
```

### Publish and clean up

Publish a workspace to an OCI registry:

```bash
nebi publish myworkspace
nebi publish myworkspace --tag v1.0.0
nebi publish myworkspace --registry ghcr --repo myorg/myenv
```

Remove and clean up workspaces:

```bash
nebi workspace remove data-science      # remove a workspace from tracking
nebi workspace remove myenv --remote   # remove a workspace from a server
nebi workspace prune                   # clean up workspaces with missing paths
```

### Connection Commands

Authenticate with a server:

```bash
nebi login https://nebi.company.com
nebi login https://nebi.company.com --token <api-token>
```

List OCI registries on the server:

```bash
nebi registry list
```

### Admin Commands

Start a server with default settings:

```bash
nebi serve
```

Or customize the port and mode:

```bash
nebi serve --port 8080 --mode server
```

### Shell Completion

Nebi supports tab completion for bash, zsh, fish, and PowerShell. Run `nebi completion --help` for setup instructions.

### Configuration

Nebi stores data in platform-standard directories:
- **Data** (`~/.local/share/nebi/`): index and credentials
- **Config** (`~/.config/nebi/config.yaml`): default server and user preferences

Run `nebi login <server-url>` to configure the server. All server-dependent commands (`push`, `pull`, `diff`, `workspace tags`, etc.) use the configured server.

## API Usage

### Authentication

```bash
# Login
curl -X POST http://localhost:8460/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"<your-password>"}'

# Returns JWT token
export TOKEN="<your-token>"
```

### Workspaces

```bash
# Create workspace
curl -X POST http://localhost:8460/api/v1/workspaces \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"myworkspace","package_manager":"pixi"}'

# List workspaces
curl http://localhost:8460/api/v1/workspaces \
  -H "Authorization: Bearer $TOKEN"

# Install packages
curl -X POST http://localhost:8460/api/v1/workspaces/{id}/packages \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"packages":["numpy","pandas"]}'
```

## Configuration

### Environment Variables

```bash
# Server configuration
NEBI_SERVER_PORT=8460
NEBI_SERVER_MODE=development
NEBI_SERVER_BASE_PATH=/proxy/8460  # Serve all routes under this path prefix (for reverse proxies)

# Database configuration
NEBI_DATABASE_DRIVER=postgres
NEBI_DATABASE_DSN="postgres://user:pass@host:5432/nebi"

# Queue configuration
NEBI_QUEUE_TYPE=valkey
NEBI_QUEUE_VALKEY_ADDR=valkey:6379

# Authentication
NEBI_AUTH_JWT_SECRET=<secret>

# Logging
NEBI_LOG_LEVEL=info
NEBI_LOG_FORMAT=json

# Admin user bootstrap (creates admin user on first startup if no users exist)
ADMIN_USERNAME=admin
ADMIN_PASSWORD=<your-secure-password>
ADMIN_EMAIL=admin@example.com  # Optional, defaults to <username>@nebi.local
```

### Reverse Proxy / Base Path

To run Nebi behind a reverse proxy at a sub-path (e.g., jupyter-server-proxy):

```bash
NEBI_SERVER_BASE_PATH=/proxy/8460 nebi serve
```

All routes (API, frontend, docs) are served under the specified prefix. For example, with `/proxy/8460`:
- Frontend: `http://host/proxy/8460/`
- API: `http://host/proxy/8460/api/v1/...`
- Docs: `http://host/proxy/8460/docs`

The CLI works without changes — just include the full path when connecting:
```bash
nebi login https://hub.example.com/proxy/8460
```

## Development

### Running the Server Locally

To develop or test the Nebi server on your machine, install the required tools and start the dev environment:

```bash
# Install development tools
make install-tools

# Run with hot reload (frontend + backend)
# Frontend dependencies will be automatically installed if needed
ADMIN_USERNAME=admin ADMIN_PASSWORD=<your-password> make dev
```

This will start:

- **Frontend dev server** at http://localhost:8461 (with hot reload)
- **Backend API** at http://localhost:8460 (with hot reload)
- **API docs** at http://localhost:8460/docs

**Admin Credentials:**
Set `ADMIN_USERNAME` and `ADMIN_PASSWORD` environment variables to create the admin user on first startup.

> **Note**: The admin user is automatically created on first startup when `ADMIN_USERNAME` and `ADMIN_PASSWORD` environment variables are set. If you start the server without these variables, no admin user will be created and you won't be able to log in.

> **Tip**: Access the app at http://localhost:8461 for the best development experience with instant hot reload of frontend changes!

### Make Targets

```bash
make help           # Show all targets
make dev            # Run with hot reload
make build          # Build binary
make test           # Run tests
make swagger        # Generate API docs
```

### Desktop App

Nebi includes a desktop application built with [Wails](https://wails.io/).

**Prerequisites:**
- Go 1.24+
- Node.js 20+
- [Wails CLI](https://wails.io/docs/gettingstarted/installation)

```bash
# Install wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Ensure Go bin is in PATH (add to ~/.zshrc or ~/.bashrc for persistence)
export PATH="$PATH:$(go env GOPATH)/bin"

# Run in development mode (with hot reload)
wails dev

# Build for production
wails build
```

**Platform-specific notes:**

- **Linux (Ubuntu 24.04+):** Requires webkit2gtk-4.1 and the `webkit2_41` build tag:
  ```bash
  sudo apt-get install libgtk-3-dev libwebkit2gtk-4.1-dev
  wails build -tags webkit2_41
  ```

- **macOS:** No additional dependencies required.

- **Windows:** No additional dependencies required.

The built application will be in `build/bin/`.

## Project Structure

```
nebi/
├── cmd/nebi/             # Unified CLI + server entry point
├── internal/
│   ├── api/              # HTTP handlers and routing
│   ├── auth/             # Authentication (JWT, basic auth)
│   ├── cliclient/        # HTTP client for CLI-to-server communication
│   ├── store/            # Local index, config, and credentials
│   ├── db/               # Database models and migrations
│   ├── executor/         # Job execution (local/docker)
│   ├── queue/            # Job queue (memory/valkey)
│   ├── server/           # Server initialization logic
│   ├── worker/           # Background job processor
│   └── pkgmgr/           # Pixi abstractions
└── frontend/             # React web UI
```

