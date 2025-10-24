# Phase 2: Package Manager Abstraction

## Overview

Phase 2 focuses on creating an abstraction layer for package managers (pixi and future uv support) and preparing container images for execution. This phase does NOT yet implement Docker/K8s runtime - that's Phase 3.

## Current State (Phase 1 Complete ✅)

The foundation is ready:
- ✅ Database with all models (including `environments`, `jobs`, `packages`)
- ✅ Job queue system (in-memory, ready for workers)
- ✅ Authentication and API framework
- ✅ Structured logging with slog
- ✅ Configuration management

## Phase 2 Goals

From `plan.md` Phase 2:
1. **Package manager interface** - Abstract pixi/uv operations (create, install, remove, list)
2. **Pixi implementation** - Pixi-specific commands and manifest parsing
3. **Future uv support** - Stub for uv backend implementation
4. **Container images** - Base images with pixi (and later uv) installed

## Implementation Plan

### 1. Package Manager Interface

Create `internal/pkgmgr/pkgmgr.go`:

```go
package pkgmgr

import "context"

// PackageManager is the interface that all package managers must implement
type PackageManager interface {
    // Name returns the package manager name (e.g., "pixi", "uv")
    Name() string

    // Init creates a new environment
    Init(ctx context.Context, opts InitOptions) error

    // Install adds packages to an environment
    Install(ctx context.Context, opts InstallOptions) error

    // Remove removes packages from an environment
    Remove(ctx context.Context, opts RemoveOptions) error

    // List returns installed packages
    List(ctx context.Context, opts ListOptions) ([]Package, error)

    // Update updates packages in an environment
    Update(ctx context.Context, opts UpdateOptions) error

    // GetManifest returns the parsed manifest file
    GetManifest(ctx context.Context, envPath string) (*Manifest, error)
}

type InitOptions struct {
    EnvPath     string   // Path where environment will be created
    Name        string   // Environment name
    Python      string   // Python version (if applicable)
    Channels    []string // Conda channels (pixi only)
}

type InstallOptions struct {
    EnvPath  string   // Path to environment
    Packages []string // Package names (e.g., "numpy==1.24.0")
}

type RemoveOptions struct {
    EnvPath  string   // Path to environment
    Packages []string // Package names to remove
}

type ListOptions struct {
    EnvPath string // Path to environment
}

type UpdateOptions struct {
    EnvPath  string   // Path to environment
    Packages []string // Packages to update (empty = update all)
}

type Package struct {
    Name    string
    Version string
    Channel string // For conda-based managers
}

type Manifest struct {
    Name     string
    Packages map[string]string // name -> version
    Channels []string
    Raw      []byte // Raw manifest content
}
```

### 2. Pixi Implementation

Create `internal/pkgmgr/pixi/pixi.go`:

**Key Pixi Commands to Implement:**
- `pixi init <name>` - Create new environment
- `pixi add <packages>` - Install packages
- `pixi remove <packages>` - Remove packages
- `pixi list` - List installed packages
- Parse `pixi.toml` manifest file

**Implementation Strategy:**
- Use `os/exec` to run pixi commands
- Parse stdout/stderr for progress and errors
- Parse `pixi.toml` (TOML format) for manifest info
- Handle exit codes and error messages
- Stream output to logs for real-time feedback

**Example Structure:**
```go
package pixi

import (
    "context"
    "os/exec"
    "github.com/aktech/darb/internal/pkgmgr"
)

type PixiManager struct {
    pixiPath string // Path to pixi binary
}

func New() (*PixiManager, error) {
    // Find pixi binary in PATH
    pixiPath, err := exec.LookPath("pixi")
    if err != nil {
        return nil, fmt.Errorf("pixi not found in PATH: %w", err)
    }
    return &PixiManager{pixiPath: pixiPath}, nil
}

func (p *PixiManager) Name() string {
    return "pixi"
}

func (p *PixiManager) Init(ctx context.Context, opts pkgmgr.InitOptions) error {
    // Implement: pixi init --channel <channels> <name>
    // Execute in opts.EnvPath
}

// ... implement other methods
```

### 3. UV Stub Implementation

Create `internal/pkgmgr/uv/uv.go`:

For now, just create a stub that returns "not implemented" errors:

```go
package uv

import (
    "context"
    "errors"
    "github.com/aktech/darb/internal/pkgmgr"
)

var ErrNotImplemented = errors.New("uv support not yet implemented")

type UvManager struct{}

func New() (*UvManager, error) {
    return &UvManager{}, nil
}

func (u *UvManager) Name() string {
    return "uv"
}

func (u *UvManager) Init(ctx context.Context, opts pkgmgr.InitOptions) error {
    return ErrNotImplemented
}

// ... return ErrNotImplemented for all methods
```

### 4. Package Manager Factory

Create `internal/pkgmgr/factory.go`:

```go
package pkgmgr

import (
    "fmt"
    "github.com/aktech/darb/internal/pkgmgr/pixi"
    "github.com/aktech/darb/internal/pkgmgr/uv"
)

// New creates a package manager instance based on type
func New(pmType string) (PackageManager, error) {
    switch pmType {
    case "pixi":
        return pixi.New()
    case "uv":
        return uv.New()
    default:
        return nil, fmt.Errorf("unsupported package manager: %s", pmType)
    }
}
```

### 5. Testing Strategy

Create `internal/pkgmgr/pixi/pixi_test.go`:

**Test with actual pixi binary if available:**
- Skip tests if pixi not in PATH
- Test Init creates pixi.toml
- Test Install adds packages
- Test List returns packages
- Test Remove removes packages
- Clean up test environments after each test

**Mock tests for CI:**
- Mock exec.Command for testing without pixi
- Test error handling
- Test output parsing

### 6. Container Images (Dockerfile)

Create `docker/pixi.Dockerfile`:

```dockerfile
FROM ubuntu:24.04

# Install pixi
RUN apt-get update && \
    apt-get install -y curl && \
    curl -fsSL https://pixi.sh/install.sh | bash && \
    rm -rf /var/lib/apt/lists/*

ENV PATH="/root/.pixi/bin:${PATH}"

# Verify installation
RUN pixi --version

WORKDIR /workspace
```

Create `docker/uv.Dockerfile` (for future):

```dockerfile
FROM python:3.12-slim

# Install uv
RUN pip install uv

# Verify installation
RUN uv --version

WORKDIR /workspace
```

### 7. Update Configuration

Update `internal/config/config.go` to add package manager config:

```go
type Config struct {
    // ... existing fields
    PackageManager PackageManagerConfig `mapstructure:"package_manager"`
}

type PackageManagerConfig struct {
    DefaultType string `mapstructure:"default_type"` // "pixi" or "uv"
    PixiPath    string `mapstructure:"pixi_path"`    // Custom pixi binary path
    UvPath      string `mapstructure:"uv_path"`      // Custom uv binary path
}
```

Update `config.yaml`:

```yaml
# ... existing config

package_manager:
  default_type: pixi
  # pixi_path: /custom/path/to/pixi  # Optional
  # uv_path: /custom/path/to/uv      # Optional
```

## File Structure After Phase 2

```
darb/
├── cmd/server/main.go
├── internal/
│   ├── api/
│   ├── auth/
│   ├── config/
│   │   └── config.go           # Updated with PackageManagerConfig
│   ├── db/
│   ├── logger/
│   ├── models/
│   ├── queue/
│   └── pkgmgr/                 # NEW
│       ├── pkgmgr.go           # Interface and types
│       ├── factory.go          # Package manager factory
│       ├── pixi/
│       │   ├── pixi.go         # Pixi implementation
│       │   └── pixi_test.go   # Tests
│       └── uv/
│           ├── uv.go           # UV stub
│           └── uv_test.go     # Tests
├── docker/                     # NEW
│   ├── pixi.Dockerfile
│   └── uv.Dockerfile
├── scripts/
│   └── create_user.go
├── config.yaml
├── Makefile
├── README.md
└── PHASE2.md                   # This file
```

## Acceptance Criteria

Phase 2 is complete when:

- [ ] Package manager interface defined with all required methods
- [ ] Pixi implementation working (can init, install, remove, list)
- [ ] UV stub created (returns not implemented errors)
- [ ] Factory function creates correct package manager instance
- [ ] Tests written for pixi operations
- [ ] Dockerfiles created for pixi and uv base images
- [ ] Configuration updated to support package manager selection
- [ ] README updated with Phase 2 status

## Testing Phase 2

**Manual Testing:**

1. Install pixi locally:
   ```bash
   curl -fsSL https://pixi.sh/install.sh | bash
   ```

2. Test package manager operations:
   ```go
   // Example test code
   pm, err := pixi.New()
   if err != nil {
       log.Fatal(err)
   }

   // Create test environment
   err = pm.Init(ctx, pkgmgr.InitOptions{
       EnvPath: "/tmp/test-env",
       Name:    "test",
       Channels: []string{"conda-forge"},
   })

   // Install packages
   err = pm.Install(ctx, pkgmgr.InstallOptions{
       EnvPath:  "/tmp/test-env",
       Packages: []string{"numpy"},
   })

   // List packages
   packages, err := pm.List(ctx, pkgmgr.ListOptions{
       EnvPath: "/tmp/test-env",
   })
   ```

3. Build Docker images:
   ```bash
   docker build -f docker/pixi.Dockerfile -t darb-pixi:latest .
   docker run -it darb-pixi:latest pixi --version
   ```

## Integration with Phase 1

The package manager abstraction integrates with existing Phase 1 components:

- **Models**: Use `models.Environment` to store package manager type
- **Jobs**: Environment operations will use this abstraction in Phase 4
- **Config**: Package manager settings in `config.yaml`
- **Logging**: Use existing `slog` for operation logs

## Notes for Implementation

1. **Error Handling**: Pixi commands can fail - capture stderr and parse error messages
2. **Context Cancellation**: Support context cancellation for long-running operations
3. **Streaming Output**: Capture stdout/stderr in real-time for job logs
4. **Path Management**: Handle environment paths carefully (absolute vs relative)
5. **TOML Parsing**: Use `github.com/pelletier/go-toml` for pixi.toml (already in go.mod from viper)
6. **Version Detection**: Detect pixi version to ensure compatibility

## Dependencies to Add

```bash
# Already available (from viper):
# - github.com/pelletier/go-toml

# No new dependencies needed for Phase 2!
# Use standard library os/exec for running commands
```

## Makefile Updates

Add to `Makefile`:

```makefile
build-docker-pixi: ## Build pixi Docker image
	docker build -f docker/pixi.Dockerfile -t darb-pixi:latest .

build-docker-uv: ## Build uv Docker image
	docker build -f docker/uv.Dockerfile -t darb-uv:latest .

build-docker: build-docker-pixi build-docker-uv ## Build all Docker images

test-pkgmgr: ## Test package manager operations
	go test -v ./internal/pkgmgr/...
```

## Questions to Resolve

Before starting Phase 2, clarify:

1. **Pixi availability**: Should pixi be required on the host, or only in containers?
   - Recommendation: Support both for flexibility

2. **Environment storage**: Where should environments be stored?
   - Local mode: `/var/lib/darb/environments/<env-id>/`
   - Container mode: Volumes (Phase 3)

3. **Manifest management**: Should we store manifest in DB or just on disk?
   - Recommendation: Store path in DB, read from disk when needed

4. **Concurrency**: Can multiple package operations run on same environment?
   - Recommendation: No - use job queue to serialize operations per environment

## Success Metrics

Phase 2 is successful if:
- ✅ Can create a pixi environment programmatically
- ✅ Can install/remove packages in that environment
- ✅ Can list installed packages
- ✅ Can parse pixi.toml manifest
- ✅ Docker image builds successfully
- ✅ UV stub exists for future implementation
- ✅ Tests pass (or skip gracefully if pixi not available)

---

**Ready to start Phase 2?** Begin with defining the interface in `internal/pkgmgr/pkgmgr.go`, then implement pixi operations!
