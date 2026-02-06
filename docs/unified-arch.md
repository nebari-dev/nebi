# Nebi Unified Architecture Plan

## Overview

Simplify Nebi to a single-server model and unify the desktop/server web UI with mode-based feature visibility.

**Primary use case**: Individual developer connecting to ONE Nebi server, pulling environments locally.

**Note**: No backward compatibility needed - clean break from current multi-server design.

---

## Phase 1: Single-Server CLI Simplification

**Goal**: Remove multi-server complexity. One server URL, no `-s` flags.

### Data Model Changes

**`internal/localstore/types.go`**:
```go
// Before
type Index struct {
    Workspaces map[string]*Workspace
    Servers    map[string]string      // name -> URL
}
type Workspace struct {
    Origins map[string]*Origin        // server name -> origin
}

// After
type Index struct {
    Workspaces map[string]*Workspace
    ServerURL  string                 // single server
}
type Workspace struct {
    Origin *Origin                    // single origin
}
```

**`internal/localstore/credentials.go`**:
```go
// Before
type Credentials struct {
    Servers map[string]*ServerCredential  // URL -> creds
}

// After
type Credentials struct {
    Token    string
    Username string
}
```

### Commands to Modify

| Command | Change |
|---------|--------|
| `cmd/nebi/server.go` | **Delete entirely** |
| `cmd/nebi/login.go` | `nebi login <url>` sets server URL AND authenticates in one step |
| `cmd/nebi/push.go` | Remove `-s` flag |
| `cmd/nebi/pull.go` | Remove `-s` flag |
| `cmd/nebi/diff.go` | Remove `-s` flag |
| `cmd/nebi/publish.go` | Remove `-s` flag |
| `cmd/nebi/workspace.go` | Remove `-s` from list/tags/remove |
| `cmd/nebi/registry.go` | Remove `-s` flag |
| `cmd/nebi/client.go` | Simplify helpers: `getServerURL()`, `getAuthenticatedClient()` |

### Files to Modify
- `internal/localstore/types.go` - simplify data structures
- `internal/localstore/store.go` - remove multi-server logic
- `internal/localstore/credentials.go` - single credential, not map
- `internal/localstore/config.go` - remove DefaultServer
- `cmd/nebi/server.go` - **DELETE**
- `cmd/nebi/login.go` - now sets server URL + authenticates
- `cmd/nebi/push.go`, `pull.go`, `diff.go`, `publish.go` - remove `-s` flag
- `cmd/nebi/workspace.go` - remove `-s` flag
- `cmd/nebi/registry.go` - remove `-s` flag
- `cmd/nebi/client.go` - simplify helpers
- `cmd/nebi/main.go` - remove server subcommand registration

---

## Phase 2: Mode-Based Frontend UI

**Goal**: Single frontend with `local` vs `team` mode. Hide multi-user features in local mode.

### Backend: Add Mode to Config

**`internal/config/config.go`**:
```go
type ServerConfig struct {
    Mode string `env:"NEBI_MODE" envDefault:"team"` // "local" or "team"
    // ...existing fields
}
```

**`internal/api/handlers/version.go`** - Add mode to response:
```json
GET /api/v1/version
{
  "version": "1.0.0",
  "mode": "local",
  "features": {
    "authentication": false,
    "userManagement": false,
    "auditLogs": false
  }
}
```

**`app.go`** - Set local mode for desktop:
```go
os.Setenv("NEBI_MODE", "local")
```

### Backend: Bypass Auth for Local Mode

**`internal/api/middleware/auth.go`** or **`internal/api/router.go`**:
- Add middleware that checks if `Mode == "local"`
- If local mode AND request is from localhost: skip JWT validation
- Create a synthetic admin user context for the request
- All API calls succeed without authentication

### Frontend: Mode Store

**New file `frontend/src/store/modeStore.ts`**:
```typescript
interface ModeState {
  mode: 'local' | 'team' | 'loading';
  features: {
    authentication: boolean;
    userManagement: boolean;
    auditLogs: boolean;
  };
}
```

### Frontend: Conditional Rendering

**`frontend/src/App.tsx`**:
- Fetch `/api/v1/version` on startup, populate mode store
- If local mode: skip login, auto-authenticate
- If team mode: show login page

**`frontend/src/components/layout/Layout.tsx`**:
- Already has `{isAdmin && <AdminNav>}`
- Add: `{mode === 'team' && isAdmin && <AdminNav>}`

**`frontend/src/pages/Login.tsx`**:
- If local mode: redirect to `/environments` immediately

### Features Hidden in Local Mode

| Feature | Local Mode | Team Mode |
|---------|-----------|-----------|
| Login page | Skip | Show |
| Admin nav | Hide | Show if admin |
| User management | Hide | Show if admin |
| Audit logs | Hide | Show if admin |
| Collaborators/sharing | Hide | Show |

### Files to Modify
- `internal/config/config.go`
- `internal/api/handlers/version.go`
- `internal/api/router.go` (add local mode auth bypass)
- `app.go`
- `frontend/src/store/modeStore.ts` (new)
- `frontend/src/App.tsx`
- `frontend/src/components/layout/Layout.tsx`
- `frontend/src/pages/Login.tsx`

---

## Phase 3: Local Storage SQLite Migration

**Goal**: Migrate CLI local storage from JSON to SQLite for consistency.

### Schema
```sql
CREATE TABLE config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    server_url TEXT,
    updated_at DATETIME
);

CREATE TABLE credentials (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    token TEXT,
    username TEXT,
    updated_at DATETIME
);

CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    path TEXT UNIQUE NOT NULL,
    is_global BOOLEAN DEFAULT FALSE,
    origin_name TEXT,
    origin_tag TEXT,
    origin_action TEXT,
    origin_toml_hash TEXT,
    origin_lock_hash TEXT,
    origin_timestamp DATETIME,
    created_at DATETIME
);
```

### Files to Modify/Create
- `internal/localstore/sqlite.go` (new - SQLite implementation)
- `internal/localstore/store.go` (switch to SQLite backend, delete JSON logic)
- Delete: `internal/localstore/config.go` (merge into store)
- Delete: `internal/localstore/credentials.go` (merge into SQLite)
- All CLI commands that use localstore (should work unchanged via interface)

---

## Implementation Order

**Phase 1** (Single-Server CLI) - Do first
- Prerequisite for cleaner architecture
- ~2-3 days

**Phase 2** (Mode-Based UI) - Can parallel with Phase 1
- Backend changes independent of CLI
- Frontend changes can be tested with existing server
- ~2-3 days

**Phase 3** (SQLite Migration) - After Phase 1
- Depends on simplified data model from Phase 1
- ~2 days

**Total estimate**: ~6-8 days

---

## Testing Strategy

### Phase 1
- Integration tests for CLI commands without `-s`
- Manual: `nebi login <url>`, `nebi push/pull`

### Phase 2
- Unit tests for mode endpoint
- Frontend tests for mode store
- E2E: Desktop app loads without login, server requires login

### Phase 3
- Unit tests for SQLite CRUD operations
- Verify CLI commands work with new storage

---

## Decisions Made

1. **Server command**: `nebi login <url>` does both - sets server URL AND authenticates in one step
2. **Local mode auth**: Bypass auth entirely for localhost requests (simplest approach)
3. **SQLite migration**: Include as Phase 3 (do it now for consistency)

---

## New CLI UX After Changes

```bash
# Connect to a server (sets URL + authenticates)
nebi login https://nebi.company.com

# All commands use the configured server (no -s flag)
nebi push myenv:v1.0
nebi pull myenv:v1.0
nebi diff myenv:v1.0 myenv:v2.0
nebi workspace list
nebi workspace tags myenv

# Check current server
nebi status   # shows server URL + workspace sync status
```