# Phase 9: Single Binary Packaging Plan

## Overview

Package the entire Darb application (Go backend + React frontend) into a single, self-contained binary for easy deployment. This eliminates the need for separate frontend builds and simplifies distribution.

---

## Current Architecture

### Backend
- **Language**: Go
- **Location**: `cmd/server/main.go`
- **Port**: 8080
- **API**: `/api/v1/*`
- **Dependencies**: SQLite database, Pixi package manager

### Frontend
- **Framework**: React + Vite + TypeScript
- **Location**: `frontend/`
- **Build Output**: `frontend/dist/`
- **Build Command**: `npm run build`
- **Assets**: Static HTML, CSS, JS files

### Current Deployment
- Backend and frontend are separate
- Frontend must be built before backend starts
- Backend currently doesn't serve frontend files
- Requires reverse proxy (nginx) or separate hosting for frontend

---

## Goals

1. **Embed frontend assets in Go binary** using `embed` package
2. **Single binary deployment** - one file contains everything
3. **Simplified build process** - automated frontend build + Go compile
4. **Cross-platform releases** - binaries for Linux, macOS, Windows
5. **Docker support** - containerized single binary
6. **Kubernetes manifests** - ready for k8s deployment
7. **Version management** - proper semantic versioning

---

## Implementation Plan

### Step 1: Embed Frontend in Go Binary

**Goal**: Use Go's `embed` package to bundle the React build into the binary.

**Files to Create/Modify**:
- `internal/web/embed.go` - Embed directive and file server setup
- `cmd/server/main.go` - Add frontend serving routes

**Implementation**:

```go
// internal/web/embed.go
package web

import (
    "embed"
    "io/fs"
    "net/http"
)

//go:embed dist/*
var frontendFS embed.FS

// GetFileSystem returns the embedded filesystem
func GetFileSystem() (http.FileSystem, error) {
    dist, err := fs.Sub(frontendFS, "dist")
    if err != nil {
        return nil, err
    }
    return http.FS(dist), nil
}
```

**Router Updates** (`internal/api/router.go`):
- Add catch-all route for frontend after API routes
- Serve `index.html` for all non-API routes (SPA support)
- Serve static assets with proper MIME types

**Example**:
```go
// Serve embedded frontend
embedFS, err := web.GetFileSystem()
if err != nil {
    log.Fatal("Failed to load embedded frontend:", err)
}

// API routes first
api := router.Group("/api/v1")
// ... existing API routes

// Frontend static files
router.StaticFS("/assets", embedFS)

// SPA fallback - serve index.html for all other routes
router.NoRoute(func(c *gin.Context) {
    // Don't serve HTML for API calls
    if strings.HasPrefix(c.Request.URL.Path, "/api") {
        c.JSON(404, gin.H{"error": "Not found"})
        return
    }

    // Serve index.html from embedded FS
    c.FileFromFS("/", embedFS)
})
```

---

### Step 2: Update Build Process

**Goal**: Automate frontend build before embedding.

**Create** `Makefile`:
```makefile
.PHONY: build-frontend build-backend build clean dev

# Variables
BINARY_NAME=darb
FRONTEND_DIR=frontend
BUILD_DIR=bin
VERSION=$(shell git describe --tags --always --dirty)
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

# Build frontend and backend
build: build-frontend build-backend

# Build frontend
build-frontend:
	@echo "Building frontend..."
	cd $(FRONTEND_DIR) && npm install && npm run build
	@echo "Copying frontend build to internal/web/dist..."
	rm -rf internal/web/dist
	cp -r $(FRONTEND_DIR)/dist internal/web/dist

# Build backend with embedded frontend
build-backend:
	@echo "Building backend with embedded frontend..."
	mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -rf internal/web/dist
	rm -rf $(FRONTEND_DIR)/dist

# Development mode (without embedding)
dev:
	go run ./cmd/server

# Run tests
test:
	go test ./...

# Cross-platform builds
build-all: build-frontend
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/server
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/server
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/server
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/server
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/server
```

---

### Step 3: Frontend Build Configuration

**Update** `frontend/.env.production`:
```env
# API base URL for production (relative path)
VITE_API_URL=/api/v1
```

**Ensure** `frontend/src/api/client.ts` uses the env variable:
```typescript
const API_BASE_URL = import.meta.env.VITE_API_URL || '/api/v1';
```

---

### Step 4: Version Information

**Add version endpoint** (`internal/api/handlers/version.go`):
```go
package handlers

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

var Version = "dev" // Set via ldflags at build time

func GetVersion(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "version": Version,
        "go_version": runtime.Version(),
    })
}
```

**Update** `cmd/server/main.go`:
```go
var Version = "dev" // Set via ldflags

func main() {
    log.Printf("Starting Darb v%s", Version)
    // ... rest of main
}
```

---

### Step 5: Docker Support

**Create** `Dockerfile`:
```dockerfile
# Build stage
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Go build stage
FROM golang:1.23-alpine AS backend-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/frontend/dist ./internal/web/dist
RUN CGO_ENABLED=1 go build -ldflags="-s -w -X main.Version=$(git describe --tags --always)" -o /darb ./cmd/server

# Final stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates sqlite pixi
WORKDIR /app
COPY --from=backend-builder /darb /app/darb

# Create data directory for SQLite
RUN mkdir -p /app/data

# Expose port
EXPOSE 8080

# Environment variables
ENV DATABASE_PATH=/app/data/darb.db
ENV LOG_LEVEL=info

CMD ["/app/darb"]
```

**Create** `docker-compose.yml`:
```yaml
version: '3.8'

services:
  darb:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/data
      - ./environments:/app/data/environments
    environment:
      - DATABASE_PATH=/app/data/darb.db
      - LOG_LEVEL=info
      - GIN_MODE=release
    restart: unless-stopped
```

**Create** `.dockerignore`:
```
node_modules
frontend/dist
frontend/node_modules
bin
.git
.env
*.log
data/
environments/
```

---

### Step 6: Kubernetes Support

**Create** `k8s/deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: darb
  labels:
    app: darb
spec:
  replicas: 1
  selector:
    matchLabels:
      app: darb
  template:
    metadata:
      labels:
        app: darb
    spec:
      containers:
      - name: darb
        image: darb:latest
        ports:
        - containerPort: 8080
        env:
        - name: DATABASE_PATH
          value: /data/darb.db
        - name: LOG_LEVEL
          value: info
        - name: GIN_MODE
          value: release
        volumeMounts:
        - name: data
          mountPath: /data
        - name: environments
          mountPath: /data/environments
        livenessProbe:
          httpGet:
            path: /api/v1/health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /api/v1/health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: darb-data-pvc
      - name: environments
        persistentVolumeClaim:
          claimName: darb-environments-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: darb
spec:
  selector:
    app: darb
  ports:
  - protocol: TCP
    port: 80
    targetPort: 8080
  type: LoadBalancer
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: darb-data-pvc
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: darb-environments-pvc
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 50Gi
```

**Create** `k8s/ingress.yaml`:
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: darb-ingress
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
  - hosts:
    - darb.example.com
    secretName: darb-tls
  rules:
  - host: darb.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: darb
            port:
              number: 80
```

---

### Step 7: GitHub Actions CI/CD

**Create** `.github/workflows/release.yml`:
```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3

    - name: Set up Node.js
      uses: actions/setup-node@v3
      with:
        node-version: '20'

    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.23'

    - name: Build Frontend
      run: |
        cd frontend
        npm ci
        npm run build
        cd ..
        mkdir -p internal/web
        cp -r frontend/dist internal/web/dist

    - name: Build Binaries
      run: |
        VERSION=${GITHUB_REF#refs/tags/}
        make build-all VERSION=$VERSION

    - name: Create Release
      uses: softprops/action-gh-release@v1
      with:
        files: bin/*
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

    - name: Build and Push Docker Image
      run: |
        echo "${{ secrets.DOCKER_PASSWORD }}" | docker login -u "${{ secrets.DOCKER_USERNAME }}" --password-stdin
        docker build -t darb:${GITHUB_REF#refs/tags/} .
        docker tag darb:${GITHUB_REF#refs/tags/} darb:latest
        docker push darb:${GITHUB_REF#refs/tags/}
        docker push darb:latest
```

---

### Step 8: Documentation Updates

**Update** `README.md`:
```markdown
## Installation

### Binary Release (Recommended)

1. Download the latest binary for your platform from [Releases](https://github.com/aktech/darb/releases)
2. Make it executable: `chmod +x darb`
3. Run it: `./darb`

The server will start on `http://localhost:8080`

### Docker

```bash
docker run -p 8080:8080 -v $(pwd)/data:/app/data darb:latest
```

### Kubernetes

```bash
kubectl apply -f k8s/
```

## Building from Source

### Prerequisites
- Go 1.23+
- Node.js 20+
- Make

### Build

```bash
make build
./bin/darb
```

## Configuration

Environment variables:
- `DATABASE_PATH` - SQLite database path (default: `./darb.db`)
- `PORT` - Server port (default: `8080`)
- `LOG_LEVEL` - Log level: debug, info, warn, error (default: `info`)
- `GIN_MODE` - Gin mode: debug, release (default: `debug`)
```

---

## File Structure After Implementation

```
darb/
├── cmd/
│   └── server/
│       └── main.go                 # Entry point, version info
├── internal/
│   ├── web/
│   │   ├── embed.go               # NEW: Embed frontend
│   │   └── dist/                  # Frontend build (gitignored, copied during build)
│   ├── api/
│   │   └── router.go              # UPDATED: Add frontend routes
│   └── handlers/
│       └── version.go             # NEW: Version endpoint
├── frontend/
│   ├── dist/                      # Build output (gitignored)
│   └── .env.production            # UPDATED: API URL config
├── k8s/
│   ├── deployment.yaml            # NEW: K8s deployment
│   └── ingress.yaml               # NEW: K8s ingress
├── .github/
│   └── workflows/
│       └── release.yml            # NEW: CI/CD pipeline
├── Dockerfile                     # NEW: Multi-stage Docker build
├── docker-compose.yml             # NEW: Docker Compose
├── Makefile                       # NEW: Build automation
├── .dockerignore                  # NEW: Docker ignore
└── README.md                      # UPDATED: Installation docs
```

---

## Testing Checklist

After implementation, verify:

### Binary
- [ ] `make build` creates single binary in `bin/darb`
- [ ] Binary size is reasonable (< 100MB)
- [ ] Binary runs without external dependencies
- [ ] Frontend loads at `http://localhost:8080`
- [ ] API works at `http://localhost:8080/api/v1/*`
- [ ] Version endpoint shows correct version
- [ ] All frontend routes work (environments, admin, etc.)
- [ ] Static assets load correctly (CSS, JS, images)
- [ ] SPA routing works (refresh on any route)

### Cross-Platform
- [ ] Linux amd64 binary works
- [ ] Linux arm64 binary works
- [ ] macOS amd64 binary works (Intel)
- [ ] macOS arm64 binary works (Apple Silicon)
- [ ] Windows amd64 binary works

### Docker
- [ ] `docker build` succeeds
- [ ] Container starts and serves app
- [ ] Data persists in volume
- [ ] Environment variables work
- [ ] Health checks pass

### Kubernetes
- [ ] Deployment creates pods
- [ ] Service is accessible
- [ ] PVCs are created and bound
- [ ] Ingress routes traffic correctly
- [ ] Health probes work

---

## Potential Issues & Solutions

### Issue 1: Large Binary Size
**Problem**: Embedded frontend increases binary size significantly.

**Solutions**:
- Use `-ldflags="-s -w"` to strip debug info
- Compress assets before embedding
- Use UPX to compress the final binary
- Consider separate binary + assets approach for very large frontends

### Issue 2: SPA Routing
**Problem**: Direct navigation to frontend routes returns 404.

**Solution**: Implement catch-all route that serves `index.html` for non-API paths.

### Issue 3: MIME Types
**Problem**: Static assets served with wrong content type.

**Solution**: Use `http.FileServer` which automatically sets correct MIME types.

### Issue 4: API Proxy in Development
**Problem**: During development, frontend dev server needs to proxy API calls.

**Solution**: Keep Vite proxy config in `frontend/vite.config.ts` for dev mode only.

### Issue 5: Cache Busting
**Problem**: Browsers cache old frontend assets.

**Solution**: Vite automatically adds hash to filenames. Ensure proper cache headers.

---

## Success Criteria

Phase 9 is complete when:

1. **Single binary** contains both frontend and backend
2. **Simple deployment**: Download binary, run it, access web UI
3. **Cross-platform**: Binaries available for Linux, macOS, Windows
4. **Docker ready**: Official Docker image published
5. **K8s ready**: Working Kubernetes manifests
6. **CI/CD**: Automated releases via GitHub Actions
7. **Documentation**: Clear installation and deployment docs
8. **Version tracking**: Binary shows correct version info

---

## Next Steps: Phase 10

After Phase 9, future enhancements could include:
- Auto-update mechanism for binaries
- Plugin system for extensions
- Multi-tenancy support
- Cloud storage backends (S3, GCS)
- Metrics and monitoring (Prometheus)
- Backup and restore functionality
