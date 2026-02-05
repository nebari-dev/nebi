# Nebi

<div align="center">
  <img src="assets/nebi-icon-solid.jpg" alt="Nebi" width="400"/>
</div>

<p align="center">
  Multi-user environment management for Pixi (UV support coming soon)
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

> **⚠️ Alpha Software**: Nebi is currently in alpha. APIs and features may change without notice. Not recommended for production use.

## What is Nebi?

Nebi is a server and CLI for managing [Pixi](https://prefix.dev/) environments in multi-user settings. The server handles environment creation, versioning, and access control, while the local-first CLI lets you track workspaces, push/pull versioned specs, and diff environments across machines or teams.

> **Note**: [UV](https://github.com/astral-sh/uv) support is planned for a future release.

**Key features:**
- Server with async job queue, RBAC, and PostgreSQL/Valkey backend
- Kubernetes-native deployment with separate API/worker pods
- Local-first CLI for workspace tracking (no server required for basic use)
- Push/pull versioned `pixi.toml` and `pixi.lock` to shared servers
- Diff specs between local directories or server versions
- Multi-server support with named servers and default server config

## Quick Start

### Local Development

```bash
# Install development tools
make install-tools

# Run with hot reload (frontend + backend)
# Frontend dependencies will be automatically installed if needed
ADMIN_USERNAME=admin ADMIN_PASSWORD=admin123 make dev
```

This will start:
- **Frontend dev server** at http://localhost:8461 (with hot reload)
- **Backend API** at http://localhost:8460 (with hot reload)
- **API docs** at http://localhost:8460/docs

**Default Admin Credentials:**
- Username: `admin`
- Password: `admin123`

> **Note**: The admin user is automatically created on first startup when `ADMIN_USERNAME` and `ADMIN_PASSWORD` environment variables are set. If you start the server without these variables, no admin user will be created and you won't be able to log in.

> **Tip**: Access the app at http://localhost:8461 for the best development experience with instant hot reload of frontend changes!

### Kubernetes Deployment

```bash
# Build and import to k3d
docker build -t nebi:latest .
k3d image import nebi:latest -c nebi-dev

# Deploy
helm install nebi ./chart -n nebi --create-namespace \
  -f chart/values-dev.yaml

# Access
curl http://localhost:8460/api/v1/health
```

## Architecture

```
┌─────────────┐      ┌──────────────┐
│  API Pod    │      │ Worker Pod   │
│             │      │              │
│ HTTP :8460  │      │ Processes    │
│ Enqueues ───┼──┐   │ pixi/uv jobs │
└─────────────┘  │   └──────────────┘
                 │
          ┌──────▼──────┐
          │   Valkey    │
          │   (Queue)   │
          └──────┬──────┘
                 │
          ┌──────▼──────┐
          │ PostgreSQL  │
          └─────────────┘
```

- **API**: Handles HTTP requests, enqueues jobs
- **Worker**: Processes jobs asynchronously
- **Valkey**: Job queue
- **PostgreSQL**: Persistent storage

## API Usage

### Authentication

```bash
# Login
curl -X POST http://localhost:8460/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'

# Returns JWT token
export TOKEN="<your-token>"
```

### Environments

```bash
# Create environment
curl -X POST http://localhost:8460/api/v1/environments \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"myenv","package_manager":"pixi"}'

# List environments
curl http://localhost:8460/api/v1/environments \
  -H "Authorization: Bearer $TOKEN"

# Install packages
curl -X POST http://localhost:8460/api/v1/environments/{id}/packages \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"packages":["numpy","pandas"]}'
```

## Configuration

### Environment Variables

> **Note**: Environment variables currently use the `NEBI_` prefix. This will be renamed to `NEBI_` in a future release.

```bash
# Server configuration
NEBI_SERVER_PORT=8460
NEBI_SERVER_MODE=development

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
ADMIN_PASSWORD=admin123
ADMIN_EMAIL=admin@nebi.local  # Optional, defaults to <username>@nebi.local
```

### Helm Values

See `chart/values.yaml` for production and `chart/values-dev.yaml` for development.

**Key settings:**
```yaml
deployment:
  api:
    replicas: 2
    resources:
      limits:
        memory: 512Mi
  worker:
    replicas: 2
    resources:
      limits:
        memory: 2Gi

database:
  driver: postgres

queue:
  type: valkey
```

## CLI Usage

### Workspace Commands

```bash
# Track a pixi workspace (runs pixi init if no pixi.toml exists)
cd my-project
nebi init

# Check sync status
nebi status

# List tracked workspaces
nebi workspace list

# Compare pixi specs between directories or server versions
nebi diff                                # local vs last pushed/pulled origin
nebi diff ./project-a ./project-b
nebi diff ./project-a ./project-b --lock    # also compare pixi.lock
nebi diff myworkspace:v1 myworkspace:v2 -s work

# Push/pull versioned specs
nebi push myworkspace:v1.0 -s work
nebi push :v2.0                          # reuse workspace name from origin
nebi pull myworkspace:v1.0 -s work
nebi pull                                # re-pull from last origin

# List workspaces and tags on a server
nebi workspace list -s work
nebi workspace tags myworkspace -s work

# Global workspaces (stored centrally by nebi)
nebi pull myworkspace:v1.0 --global data-science -s work
nebi workspace promote data-science     # copy current workspace to global
nebi workspace list                     # shows local and global workspaces
nebi shell data-science                 # open pixi shell in a workspace by name
nebi shell data-science -e dev          # args pass through to pixi shell
nebi run my-task                        # run a pixi task (auto-initializes workspace)
nebi run data-science my-task           # run a task in a global workspace
nebi workspace remove data-science      # remove a workspace from tracking
nebi workspace remove myenv -s work    # remove a workspace from a server
nebi workspace prune                   # clean up workspaces with missing paths

# Diff using workspace names
nebi diff data-science ./my-project
nebi diff data-science ml-pipeline

# Publish a workspace version to an OCI registry
nebi workspace publish myworkspace:v1.0 -s work
nebi workspace publish myworkspace:v1.0 -s work myorg/myenv:latest
nebi workspace publish myworkspace:v1.0 -s work --registry ghcr myorg/myenv:latest
```

### Connection Commands

```bash
# Register and authenticate with a server
nebi server add work https://nebi.company.com
nebi login work

# Change the default server
nebi server set-default work

# List OCI registries on a server
nebi registry list -s work
```

### Admin Commands

```bash
# Run a server instance
nebi serve
nebi serve --port 8080 --mode server
```

### Configuration

Nebi stores data in platform-standard directories:
- **Data** (`~/.local/share/nebi/`): index, credentials, global workspace environments
- **Config** (`~/.config/nebi/config.yaml`): default server and user preferences

The first server added with `nebi server add` automatically becomes the default, so `-s` can be omitted on commands like `push`, `pull`, `diff`, and `workspace tags`.

## Development

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
│   ├── localstore/       # Local index, config, and credentials
│   ├── db/               # Database models and migrations
│   ├── executor/         # Job execution (local/docker/k8s)
│   ├── queue/            # Job queue (memory/valkey)
│   ├── server/           # Server initialization logic
│   ├── worker/           # Background job processor
│   └── pkgmgr/           # Pixi/UV abstractions
├── chart/                # Helm chart for Kubernetes
└── frontend/             # React web UI
```

