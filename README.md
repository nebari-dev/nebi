# Darb

<p align="center">
  <img src="assets/darb-high-resolution-logo.png" alt="Darb" width="500"/>
</p>

<p align="center">
  Multi-user environment management for Pixi (UV support coming soon)
</p>

<p align="center">
  <a href="https://github.com/openteams-ai/darb/actions/workflows/ci.yml">
    <img src="https://github.com/openteams-ai/darb/actions/workflows/ci.yml/badge.svg" alt="CI">
  </a>
  <a href="https://github.com/openteams-ai/darb/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/openteams-ai/darb" alt="License">
  </a>
  <a href="https://github.com/openteams-ai/darb/releases">
    <img src="https://img.shields.io/github/v/release/openteams-ai/darb?include_prereleases" alt="Release">
  </a>
  <a href="https://github.com/openteams-ai/darb/issues">
    <img src="https://img.shields.io/github/issues/openteams-ai/darb" alt="Issues">
  </a>
  <a href="https://github.com/openteams-ai/darb/pulls">
    <img src="https://img.shields.io/github/issues-pr/openteams-ai/darb" alt="Pull Requests">
  </a>
</p>

---

> **⚠️ Alpha Software**: Darb is currently in alpha. APIs and features may change without notice. Not recommended for production use.

## What is Darb?

Darb is a REST API and web UI for managing [Pixi](https://prefix.dev/) environments in multi-user settings. It handles environment creation, package installation, and job execution with proper isolation and access control.

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

### Desktop App

Run Darb as a native desktop application with an embedded server.

#### Recommended: Docker-based Builds

For consistent builds across different machines, use the Docker builder:

```bash
# Build the builder image (first time only)
make build-wails-builder

# Build Linux desktop app using Docker
make build-desktop-docker
```

This avoids needing to install GTK, WebKit, and other system dependencies locally.

#### Native Development (requires system dependencies)

For native development on Linux, you'll need:
```bash
# Ubuntu 24.04+
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev

# Install Wails CLI
make install-wails

# Run in development mode
make dev-desktop

# Build for current platform
make build-desktop
```

> **Note**: For daily development, `make dev` (web UI) is recommended. The desktop app wraps the same UI.

The desktop app:
- Runs an embedded HTTP server on `127.0.0.1:8460`
- Stores data in platform-specific directories:
  - **macOS**: `~/Library/Application Support/Darb/`
  - **Windows**: `%APPDATA%\Darb\`
  - **Linux**: `~/.local/share/darb/`
- Creates a default admin user (`admin`/`admin`) on first run
- Works with the CLI - just point to `http://localhost:8460`

#### CI/CD

Desktop builds for all platforms (Linux, macOS, Windows) run automatically in GitHub Actions on push to main and on releases. Binaries are attached to GitHub releases.

### Kubernetes Deployment

```bash
# Build and import to k3d
docker build -t darb:latest .
k3d image import darb:latest -c darb-dev

# Deploy
helm install darb ./chart -n darb --create-namespace \
  -f chart/values-dev.yaml

# Access
curl http://localhost:8460/api/v1/health
```

### Fly.io Deployment

Deploy to fly.io using GitHub Actions:

```bash
# Generate a deploy token for CI/CD
flyctl tokens create deploy --name github-actions-darb

# Set GitHub secrets
gh secret set FLY_API_TOKEN --body "<token-from-above>"
gh secret set JWT_SECRET --body "$(openssl rand -base64 32)"
gh secret set ADMIN_USERNAME --body "admin"
gh secret set ADMIN_PASSWORD --body "$(openssl rand -base64 24)"

# Deploy via GitHub Actions
gh workflow run deploy.yml
```

The deployment workflow will:
- Create the fly.io app (if it doesn't exist)
- Create a 1GB volume for SQLite database
- Set required secrets
- Build and deploy the Docker image

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

## Scaling

```bash
# Scale workers based on job queue depth
kubectl scale deployment darb-worker -n darb --replicas=5

# Scale API for HTTP traffic
kubectl scale deployment darb-api -n darb --replicas=3
```

## Development

```bash
make help                 # Show all targets
make dev                  # Run server with hot reload
make build                # Build server binary
make test                 # Run tests
make swagger              # Generate API docs

# Desktop app (Docker - recommended)
make build-wails-builder  # Build the Docker builder image
make build-desktop-docker # Build Linux desktop via Docker

# Desktop app (native - requires system deps)
make dev-desktop          # Run desktop app with hot reload
make build-desktop        # Build desktop app for current platform
```

## Project Structure

```
darb/
├── cmd/
│   ├── server/           # Server entry point
│   ├── darb-cli/         # CLI client
│   └── desktop/          # Desktop app (Wails main + config)
├── desktop/              # Desktop app package
│   ├── app.go            # App lifecycle, embedded server
│   ├── config.go         # Platform-specific paths
│   └── proxy.go          # HTTP proxy for WebView
├── docker/
│   └── wails-builder.Dockerfile  # Docker image for desktop builds
├── internal/
│   ├── api/              # HTTP handlers and routing
│   ├── auth/             # Authentication (JWT, basic auth)
│   ├── db/               # Database models and migrations
│   ├── executor/         # Job execution (local/docker/k8s)
│   ├── queue/            # Job queue (memory/valkey)
│   ├── worker/           # Background job processor
│   └── pkgmgr/           # Pixi/UV abstractions
├── chart/                # Helm chart for Kubernetes
└── frontend/             # React web UI
```

## License

MIT
