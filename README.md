# Nebi

<div align="center">
  <table><tr><td bgcolor="white" style="padding: 20px;">
    <img src="assets/nebi-logo.png" alt="Nebi" width="500"/>
  </td></tr></table>
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

Nebi is a REST API and web UI for managing [Pixi](https://prefix.dev/) environments in multi-user settings. It handles environment creation, package installation, and job execution with proper isolation and access control.

> **Note**: [UV](https://github.com/astral-sh/uv) support is planned for a future release and is currently in the roadmap.

**Key features:**
- Async job queue for package operations
- Role-based access control (RBAC)
- PostgreSQL + Valkey backend
- Kubernetes-native with separate API/worker deployments
- Real-time log streaming

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

> **Note**: Environment variables currently use the `DARB_` prefix. This will be renamed to `NEBI_` in a future release.

```bash
# Server configuration
DARB_SERVER_PORT=8460
DARB_SERVER_MODE=development

# Database configuration
DARB_DATABASE_DRIVER=postgres
DARB_DATABASE_DSN="postgres://user:pass@host:5432/darb"

# Queue configuration
DARB_QUEUE_TYPE=valkey
DARB_QUEUE_VALKEY_ADDR=valkey:6379

# Authentication
DARB_AUTH_JWT_SECRET=<secret>

# Logging
DARB_LOG_LEVEL=info
DARB_LOG_FORMAT=json

# Admin user bootstrap (creates admin user on first startup if no users exist)
ADMIN_USERNAME=admin
ADMIN_PASSWORD=admin123
ADMIN_EMAIL=admin@darb.local  # Optional, defaults to <username>@darb.local
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

The `nebi` binary includes both the server and CLI commands:

```bash
# Start the server
nebi serve
nebi serve --port 8080 --mode server  # API only, custom port

# Login to a server
nebi login http://localhost:8460

# Manage registries
nebi registry add ds-team ghcr.io/myorg/data-science --default
nebi registry list

# Manage workspaces
nebi workspace list
nebi workspace info myworkspace

# Push/pull environments
nebi push myworkspace:v1.0.0
nebi pull myworkspace:v1.0.0
nebi shell myworkspace
```

## Development

```bash
make help           # Show all targets
make dev            # Run with hot reload
make build          # Build binary
make test           # Run tests
make swagger        # Generate API docs
```

## Project Structure

```
nebi/
├── cmd/nebi/             # Unified CLI + server entry point
├── internal/
│   ├── api/              # HTTP handlers and routing
│   ├── auth/             # Authentication (JWT, basic auth)
│   ├── cliclient/        # Lightweight HTTP client for CLI
│   ├── db/               # Database models and migrations
│   ├── executor/         # Job execution (local/docker/k8s)
│   ├── queue/            # Job queue (memory/valkey)
│   ├── server/           # Server initialization logic
│   ├── worker/           # Background job processor
│   └── pkgmgr/           # Pixi/UV abstractions
├── chart/                # Helm chart for Kubernetes
└── frontend/             # React web UI
```

