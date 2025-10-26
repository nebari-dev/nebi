# Tilt configuration for Darb local development with k3d
# Supports both interactive development (tilt up) and CI (tilt ci)
#
# Prerequisites:
# - Create the k3d cluster first:
#   k3d cluster create -c k3d-config.yaml

# Validate that the cluster exists and is the current context
allow_k8s_contexts('k3d-darb-dev')

# Detect CI environment
is_ci = config.tilt_subcommand == "ci"

# Set default namespace (matches Helm release name)
k8s_namespace('darb')

# Build Docker image and import into k3d
custom_build(
    'darb',
    'docker build -t $EXPECTED_REF . && k3d image import $EXPECTED_REF -c darb-dev',
    ['./'],
    ignore=['./chart', './.git', './data', './docs', './.tiltignore'],
    skips_local_docker=True,  # Don't try to push to registry
    # live_update=[
    #     # Sync Go source files for faster iteration (optional)
    #     sync('./cmd', '/app/cmd'),
    #     sync('./internal', '/app/internal'),
    #     run('go build -o /app/darb ./cmd/server', trigger=['./cmd', './internal']),
    # ],
)

# Deploy using Helm chart with k8s test values (Postgres + Valkey)
k8s_yaml(helm(
    './chart',
    name='darb',
    namespace='darb',
    values=['./chart/values-k8s-test.yaml'],
))

# Group setup resources (namespace, ServiceAccount, PVCs, Secrets, etc.)
k8s_resource(
    objects=[
        'darb:namespace',
        'darb:serviceaccount',
        'darb-data:persistentvolumeclaim',
        'darb-environments:persistentvolumeclaim',
        'darb-postgres:secret',
    ],
    new_name='setup',
    labels=['setup'],
    pod_readiness='ignore',
)

# Configure PostgreSQL StatefulSet
k8s_resource(
    'darb-postgres',
    labels=['database'],
    resource_deps=['setup'],
    port_forwards='5432:5432',  # Forward to localhost:5432
)

# Configure Valkey Deployment
k8s_resource(
    'darb-valkey',
    labels=['cache'],
    resource_deps=['setup'],
    port_forwards='6379:6379',  # Forward to localhost:6379
)

# Configure main Darb deployment
k8s_resource(
    'darb',
    labels=['app'],
    resource_deps=['setup', 'darb-postgres', 'darb-valkey'],
    port_forwards='8080:8080',  # Forward to localhost:8080
)

# In CI mode, wait for deployment to be ready then exit
if is_ci:
    print("Running in CI mode - will exit after deployment is ready")
else:
    # Interactive mode - show helpful info
    print("""
╔══════════════════════════════════════════════════════════════╗
║                    Darb Dev Environment                       ║
╚══════════════════════════════════════════════════════════════╝

🚀 Starting up...

Once ready:
  • Darb UI:      http://localhost:8080
  • Tilt UI:      http://localhost:10350
  • API:          http://localhost:8080/api/v1/health
  • Swagger:      http://localhost:8080/docs
  • PostgreSQL:   localhost:5432 (darb/testpassword123)
  • Valkey:       localhost:6379

💡 Tips:
  • Edit code → Save → Tilt auto-rebuilds & redeploys
  • Edit Helm chart → Tilt auto-updates manifests
  • Press 'space' to open Tilt UI in browser
  • Press 'Ctrl+C' to stop

🗄️  Using PostgreSQL 18 + Valkey 9.0 (latest versions!)
📚 Data persisted in PVCs (k3s local-path)
📦 Chart location: ./chart/
""")
