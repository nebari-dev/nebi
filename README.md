# Darb

<p align="center">
  <img src="assets/darb-high-resolution-logo.png" alt="Darb" width="300"/>
</p>

<p align="center">
  Multi-user environment management for Pixi and UV
</p>

---

## What is Darb?

Darb is a REST API and web UI for managing [Pixi](https://prefix.dev/) and [UV](https://github.com/astral-sh/uv) environments in multi-user environments. It handles environment creation, package installation, and job execution with proper isolation and access control.

**Key features:**
- Async job queue for package operations
- Role-based access control (RBAC)
- PostgreSQL + Valkey backend
- Kubernetes-native with separate API/worker deployments
- Real-time log streaming

## Quick Start

### Local Development

```bash
# Install dependencies
make install-tools

# Run with hot reload
make dev

# API available at http://localhost:8080
# Docs at http://localhost:8080/docs
```

### Kubernetes Deployment

```bash
# Build and import to k3d
docker build -t darb:latest .
k3d image import darb:latest -c darb-dev

# Deploy
helm install darb ./chart -n darb --create-namespace \
  -f chart/values-dev.yaml

# Access
curl http://localhost:8080/api/v1/health
```

## Architecture

```
┌─────────────┐      ┌──────────────┐
│  API Pod    │      │ Worker Pod   │
│             │      │              │
│ HTTP :8080  │      │ Processes    │
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
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'

# Returns JWT token
export TOKEN="<your-token>"
```

### Environments

```bash
# Create environment
curl -X POST http://localhost:8080/api/v1/environments \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"myenv","package_manager":"pixi"}'

# List environments
curl http://localhost:8080/api/v1/environments \
  -H "Authorization: Bearer $TOKEN"

# Install packages
curl -X POST http://localhost:8080/api/v1/environments/{id}/packages \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"packages":["numpy","pandas"]}'
```

## Configuration

### Environment Variables

```bash
DARB_SERVER_PORT=8080
DARB_DATABASE_DRIVER=postgres
DARB_DATABASE_DSN="postgres://user:pass@host:5432/darb"
DARB_QUEUE_TYPE=valkey
DARB_QUEUE_VALKEY_ADDR=valkey:6379
DARB_AUTH_JWT_SECRET=<secret>
DARB_LOG_LEVEL=info
DARB_LOG_FORMAT=json
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
make help           # Show all targets
make dev            # Run with hot reload
make build          # Build binary
make test           # Run tests
make swagger        # Generate API docs
```

## Project Structure

```
darb/
├── cmd/server/           # Application entry point
├── internal/
│   ├── api/              # HTTP handlers and routing
│   ├── auth/             # Authentication (JWT, basic auth)
│   ├── db/               # Database models and migrations
│   ├── executor/         # Job execution (local/docker/k8s)
│   ├── queue/            # Job queue (memory/valkey)
│   ├── worker/           # Background job processor
│   └── packagemanager/   # Pixi/UV abstractions
├── chart/                # Helm chart for Kubernetes
└── frontend/             # React web UI
```

## License

MIT
