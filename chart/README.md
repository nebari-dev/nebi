# Nebi Helm Chart

Helm chart for deploying Nebi (Dynamic Arbitrary Runtime Bundle) to Kubernetes.

## Installation

### Local Development with Tilt

```bash
# Create k3d cluster
k3d cluster create -c k3d-config.yaml

# Deploy with Tilt
tilt up
```

### Direct Helm Installation

```bash
# Install with default values
helm install nebi ./chart --create-namespace --namespace nebi

# Install with custom values
helm install nebi ./chart -f custom-values.yaml --namespace nebi
```

## Configuration

### Key Values

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Image repository | `nebi` |
| `image.tag` | Image tag | `latest` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `replicaCount` | Number of replicas | `1` |
| `service.type` | Service type | `ClusterIP` |
| `service.port` | Service port | `80` |
| `persistence.enabled` | Enable persistent storage | `true` |
| `persistence.data.size` | Database volume size | `5Gi` |
| `persistence.environments.size` | Environments volume size | `20Gi` |
| `resources.requests.memory` | Memory request | `128Mi` |
| `resources.limits.memory` | Memory limit | `512Mi` |

### Environment-Specific Values

**Development** (`values-dev.yaml`):
- Uses `NodePort` service type (port 30080)
- `imagePullPolicy: Never` (local images)
- Debug logging enabled
- Reduced resource limits

**Production** (`values.yaml`):
- Uses `ClusterIP` service type
- `imagePullPolicy: IfNotPresent`
- Info-level logging
- Production resource limits
- Security contexts enforced

## Examples

### Update an existing installation

```bash
helm upgrade nebi ./chart --namespace nebi
```

### Customize storage class

```bash
helm install nebi ./chart \
  --set persistence.data.storageClass=fast-ssd \
  --namespace nebi
```

### Change resource limits

```bash
helm install nebi ./chart \
  --set resources.limits.memory=1Gi \
  --set resources.limits.cpu=1000m \
  --namespace nebi
```

### Enable ingress

```bash
helm install nebi ./chart \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=nebi.example.com \
  --namespace nebi
```

## Uninstallation

```bash
helm uninstall nebi --namespace nebi
```

## Development

### Validate chart

```bash
helm lint ./chart
```

### Render templates

```bash
helm template nebi ./chart -f ./chart/values-dev.yaml
```

### Package chart

```bash
helm package ./chart
```
