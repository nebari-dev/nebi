# Deploying Darb on k3d with Tilt + Helm

## Quick Start

Just run these two commands:

```bash
# 1. Create k3d cluster (one-time setup)
k3d cluster create -c k3d-config.yaml

# 2. Deploy with Tilt (handles everything automatically)
tilt up
```

That's it!

## What happens?

**Step 1** creates a k3d cluster named `darb-dev` with:
- Port 8080 forwarded for Darb UI
- Shared filesystem mounted at `./data/environments`

**Step 2** - Tilt will:
- Render Helm chart with dev values (`chart/values-dev.yaml`)
- Build the Docker image (multi-stage with distroless base)
- Deploy to k3d cluster
- Set up persistent storage
- Auto-reload on code or chart changes

## Access Darb

Once ready (check Tilt UI at http://localhost:10350):
- **Darb UI**: http://localhost:8080
- **API Health**: http://localhost:8080/api/v1/health
- **API Docs**: http://localhost:8080/docs

### Default Admin Credentials (Development Only)
- **Username**: `admin`
- **Password**: `admin123`
- **Email**: `admin@darb.local`

> ⚠️ **Security Warning**: Change these credentials in production! Set custom values in `chart/values.yaml` or use environment variables.

## What Tilt Does

- **Helm templating**: Renders chart with dev-specific values
- **Automatic rebuilds**: Edit any Go, frontend, or chart files → Tilt rebuilds → Redeploys
- **Live logs**: See all pod logs in Tilt UI
- **Resource dashboard**: Monitor CPU/memory usage
- **Port forwarding**: Automatic forwarding to localhost:8080

## Architecture

### Helm Chart Approach

We use **Helm as a templating engine** (via Tilt's `helm()` function):
- ✅ Single source of truth for k8s manifests
- ✅ Production-ready chart can be published later
- ✅ Environment-specific values files (dev vs prod)
- ✅ Fast iteration - Tilt watches chart changes

### File Structure

```
darb/
├── chart/                       # Helm chart
│   ├── Chart.yaml              # Chart metadata
│   ├── values.yaml             # Production defaults
│   ├── values-dev.yaml         # Development overrides
│   └── templates/
│       ├── _helpers.tpl        # Template helpers
│       ├── namespace.yaml      # Namespace
│       ├── deployment.yaml     # Deployment with probes
│       ├── service.yaml        # Service (ClusterIP/NodePort)
│       └── pvc.yaml           # Persistent volumes
├── Tiltfile                    # Tilt configuration
├── k3d-config.yaml            # k3d cluster config
├── Dockerfile                  # Multi-stage build
└── data/
    └── environments/           # Pixi environments (persistent)
```

## Environment Configuration

### Development (values-dev.yaml)
- `service.type: NodePort` - Direct access via port 30080
- `image.pullPolicy: Never` - Use local images
- `config.logLevel: debug` - Verbose logging
- `persistence.storageClass: local-path` - k3s default storage

### Production (values.yaml)
- `service.type: ClusterIP` - Internal cluster access
- `image.pullPolicy: IfNotPresent` - Pull from registry
- `config.logLevel: info` - Standard logging
- Security contexts and resource limits configured

## Advanced Usage

### Deploy to Production Kubernetes

```bash
# Package the chart
helm package ./chart

# Install to production cluster
helm install darb ./darb-0.1.0.tgz \
  --namespace darb \
  --create-namespace \
  --values values-prod.yaml
```

### Customize Values

Edit `chart/values-dev.yaml` to change:
- Resource limits
- Storage sizes
- Log levels
- Port configurations

Tilt will automatically reload when you save changes.

### Fast Iteration (Optional)

Uncomment the `live_update` section in Tiltfile for near-instant reloads:

```python
live_update=[
    sync('./cmd', '/app/cmd'),
    sync('./internal', '/app/internal'),
    run('go build -o /app/darb ./cmd/server', trigger=['./cmd', './internal']),
]
```

## Cleanup

```bash
# Stop Tilt (Ctrl+C), then:
k3d cluster delete darb-dev
```

## Features

✅ **Helm-based deployment** - Production-ready manifests
✅ **Environment parity** - Same chart, different values
✅ **Ultra-lightweight Docker image** - Distroless base (no shell)
✅ **Single binary** - Frontend embedded in Go binary
✅ **Persistent storage** - SQLite DB and pixi environments
✅ **Auto-reload** - Edit code/chart → Tilt rebuilds → Instant deployment
✅ **Security** - Runs as non-root user (uid: 65532)
✅ **Multi-arch support** - Builds for amd64/arm64 automatically

## Troubleshooting

### Check Tilt logs
Press `space` in Tilt terminal to open web UI, or visit http://localhost:10350

### Check pod status
```bash
kubectl get pods -n darb
```

### View pod logs
```bash
kubectl logs -n darb deployment/darb -f
```

### Rebuild from scratch
```bash
# In Tilt UI, click on 'darb' resource → Click 'Rebuild'
# Or restart Tilt: Ctrl+C, then tilt up
```

### Validate Helm chart
```bash
# Render templates locally
helm template darb ./chart -f ./chart/values-dev.yaml

# Check for issues
helm lint ./chart
```

### Test Helm chart without Tilt
```bash
# Install directly via Helm
helm install darb ./chart \
  --namespace darb \
  --create-namespace \
  --values ./chart/values-dev.yaml
```

## Why Helm + Tilt?

**Helm** provides:
- Production-ready package format
- Templating and value overrides
- Version management
- Easy distribution

**Tilt** adds:
- Fast local development loop
- Live reloading
- Resource monitoring
- Better DX for iteration

Together they provide the **best of both worlds**:
- Dev: Fast iteration with Tilt
- Prod: Deploy Helm chart to any k8s cluster
