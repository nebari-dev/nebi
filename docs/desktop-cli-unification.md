# Desktop App + CLI Workspace Unification

## Problem

`nebi ws list` (CLI) shows workspaces created via `nebi init`, but the **desktop app does not show them**. The goal is for CLI-created workspaces to appear in the desktop UI and vice versa.

## Current Architecture

### Shared Database

Both CLI and desktop app use the **same SQLite file**: `~/.local/share/nebi/nebi.db` on Linux.

- **CLI store** (`internal/store/store.go`): Opens via `gorm.Open(sqlite.Open(dbPath))`, runs `AutoMigrate` on `models.Workspace`, `store.Config`, `store.Credentials`.
- **Desktop app** (`app.go` → `internal/db/db.go`): Opens the same file via `db.New()`, runs `db.Migrate()` which auto-migrates the full schema (User, Role, Workspace, Job, Permission, etc.) and seeds default roles + admin user.

**Note on macOS**: `app.go:getAppDataDir()` uses `~/Library/Application Support/Nebi` (capital N) while `store.go:DefaultDataDir()` uses `~/Library/Application Support/nebi` (lowercase). These are different paths on case-sensitive filesystems. On Linux both use `~/.local/share/nebi` so they match.

### How the Desktop Frontend Fetches Workspaces

The React frontend calls the **HTTP API**, not Wails bindings:

```
frontend/src/hooks/useEnvironments.ts → useWorkspaces()
  → workspacesApi.list()                        (frontend/src/api/environments.ts)
    → apiClient.get('/workspaces')              (HTTP GET /api/v1/workspaces)
      → WorkspaceHandler.ListWorkspaces()       (internal/api/handlers/workspace.go)
```

The Wails bindings in `app.go` (`App.ListWorkspaces()`) exist but the React frontend doesn't use them — it uses the embedded HTTP API server.

### The Root Cause

`WorkspaceHandler.ListWorkspaces()` in `internal/api/handlers/workspace.go` line 46:

```go
func (h *WorkspaceHandler) ListWorkspaces(c *gin.Context) {
    userID := getUserID(c)
    query := h.db.Where("owner_id = ?", userID)
    // ... also checks permissions table
    query.Preload("Owner").Order("created_at DESC").Find(&workspaces)
}
```

This filters workspaces by `owner_id`. In local mode, `LocalAuthenticator` (`internal/auth/local.go`) injects the first admin user from the `users` table into the request context. So `getUserID(c)` returns the admin user's UUID.

**CLI-created workspaces have `owner_id` set to the zero UUID** (`00000000-0000-0000-0000-000000000000`) because the CLI's `store.CreateWorkspace()` never sets an owner. The admin user has a different UUID, so the filter excludes CLI workspaces.

## Data Flow Diagram

```
CLI (nebi init)
  → store.CreateWorkspace(ws)         # owner_id = uuid.Nil (zero UUID)
  → writes to ~/.local/share/nebi/nebi.db

Desktop App startup
  → db.New() opens same nebi.db
  → db.Migrate() adds User/Role/etc tables
  → db.CreateDefaultAdmin() creates admin user (random UUID)
  → starts embedded HTTP server on :8460

React Frontend
  → GET /api/v1/workspaces
  → LocalAuthenticator injects admin user (UUID: abc-123-...)
  → ListWorkspaces: WHERE owner_id = 'abc-123-...'
  → CLI workspaces have owner_id = '00000000-...' → NOT RETURNED
```

## The Workspace Model

`internal/models/workspace.go`:

```go
type Workspace struct {
    ID             uuid.UUID       `gorm:"type:text;primary_key" json:"id"`
    Name           string          `gorm:"not null" json:"name"`
    OwnerID        uuid.UUID       `gorm:"type:text;not null;index" json:"owner_id"`
    Owner          User            `gorm:"foreignKey:OwnerID" json:"owner,omitempty"`
    Status         WorkspaceStatus `gorm:"not null;default:'pending'" json:"status"`
    PackageManager string          `gorm:"not null" json:"package_manager"`
    SizeBytes      int64           `gorm:"default:0" json:"size_bytes"`
    Path           string          `gorm:"" json:"path,omitempty"`
    Source         string          `gorm:"default:'managed'" json:"source"`
    IsGlobal       bool            `gorm:"default:false" json:"is_global,omitempty"`
    OriginName     string          `json:"origin_name,omitempty"`
    OriginTag      string          `json:"origin_tag,omitempty"`
    OriginAction   string          `json:"origin_action,omitempty"`
    OriginTomlHash string          `json:"origin_toml_hash,omitempty"`
    OriginLockHash string          `json:"origin_lock_hash,omitempty"`
    // ... timestamps, soft delete
}
```

Key fields:
- `Source`: `"local"` (CLI-created via `nebi init`) or `"managed"` (created via desktop UI/API)
- `Path`: filesystem path for local workspaces (e.g., `/home/user/my-project`)
- `IsGlobal`: true for `nebi pull --global` workspaces
- `Origin*`: tracks last push/pull server origin

## Fix Options

### Option A: In local mode, return ALL workspaces (Recommended)

In `internal/api/handlers/workspace.go`, change `ListWorkspaces` to skip the owner filter when in local mode:

```go
func (h *WorkspaceHandler) ListWorkspaces(c *gin.Context) {
    var workspaces []models.Workspace

    if isLocalMode(c) {
        // Local/desktop mode: show all workspaces (CLI + managed)
        h.db.Order("created_at DESC").Find(&workspaces)
    } else {
        // Team mode: filter by owner + permissions
        userID := getUserID(c)
        query := h.db.Where("owner_id = ?", userID)
        // ... existing permission logic
        query.Preload("Owner").Order("created_at DESC").Find(&workspaces)
    }

    c.JSON(http.StatusOK, workspaces)
}
```

You'd need a way to detect local mode. Options:
- Check a context value set by the router/middleware
- Pass the config mode to the handler
- Check if the authenticator is `LocalAuthenticator`

The simplest: add `isLocal bool` to `WorkspaceHandler` and set it when creating the handler in `router.go`.

### Option B: CLI assigns ownership to the local admin user

Have the CLI look up the admin user and set `OwnerID` when creating workspaces. This is more complex because the CLI store doesn't currently know about the `users` table.

### Option C: Hybrid — show unowned workspaces in local mode

In local mode, modify the query to also include workspaces with zero `owner_id`:

```go
query := h.db.Where("owner_id = ? OR owner_id = ?", userID, uuid.Nil)
```

This is the smallest change but feels like a hack.

## Secondary Issues to Address

### 1. macOS Path Mismatch

`app.go:getAppDataDir()` returns `~/Library/Application Support/Nebi` (capital N).
`store.go:DefaultDataDir()` returns `~/Library/Application Support/nebi` (lowercase n).

Fix: Make them consistent (use lowercase `nebi` everywhere).

### 2. Frontend Still Shows "Environments" in Some Places

Files that still use old naming in filenames (functionality works, just naming):
- `frontend/src/api/environments.ts` (exports `workspacesApi` but filename says environments)
- `frontend/src/hooks/useEnvironments.ts` (exports `useWorkspaces` but filename says environments)
- `frontend/src/pages/Environments.tsx` (component works correctly, just filename)

These are cosmetic but should be renamed for consistency.

### 3. Wails Bindings Are Redundant

`app.go` has `ListWorkspaces()`, `CreateWorkspace()`, `DeleteWorkspace()`, `GetWorkspace()` methods that duplicate what the API does. The frontend uses the API. These could be removed or kept as a fallback.

## Files to Modify

| File | Change |
|---|---|
| `internal/api/handlers/workspace.go` | Skip owner filter in local mode |
| `internal/api/router.go` | Pass local mode flag to WorkspaceHandler |
| `app.go` | Fix data dir casing for macOS (optional) |
| `internal/store/store.go` | Ensure DefaultDataDir matches app.go (optional) |

## Verification Steps

1. `go vet ./...` — clean
2. `go test ./internal/store/... ./cmd/nebi/...` — pass
3. Run desktop app, run `nebi init` in a directory, confirm it appears in the UI
4. Create workspace via desktop UI, confirm `nebi ws list` shows it
5. `cd frontend && npx tsc --noEmit && npm run build` — clean
