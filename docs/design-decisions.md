# Key Design Decisions: Unified Architecture

This document captures the major architectural decisions made during the unified-arch branch development.

## 1. Environment to Workspace Rename

All models, handlers, API routes, frontend types/pages renamed from "environment" to "workspace." API routes changed from `/environments` to `/workspaces`, RBAC resource prefix from `env:` to `ws:`. Straightforward but touched ~110 files.

## 2. Single-Server Model (vs Multi-Server)

Each CLI/desktop instance connects to one server at a time.

Previously the workspace model had `Origins map[string]*Origin` keyed by server name. Now it uses flat fields: `OriginName`, `OriginTag`, `OriginAction`, `OriginTomlHash`, `OriginLockHash`. This eliminated the `-s <server>` flag from all CLI commands and simplified the mental model to match npm/docker (one registry at a time) rather than git's multi-remote approach. `nebi login <url>` sets the server and authenticates in one step.

## 3. Dual-Mode Operation: Local vs Team

Single codebase with `NEBI_MODE` environment variable (`local` or `team`, default `team`).

| Feature | Local (Desktop) | Team (Server) |
|---------|-----------------|---------------|
| Auth | Bypassed entirely | JWT/OIDC required |
| Workspace listing | Returns ALL workspaces | Filtered by owner + RBAC |
| Admin routes | Hidden in frontend | Available to admins |
| Remote proxy API | `/api/v1/remote/*` registered | Not registered |
| User context | Auto-injects first admin user | Validates JWT |

The frontend fetches mode from `GET /api/v1/version` and adapts (skip login page, hide admin nav, etc.).

## 4. Local Auth Bypass

In local mode, `LocalAuthenticator` finds the first user in DB and injects it into every request context -- no token validation, no password checks. Login returns a mock token `"local-mode"`. Single-user desktop app doesn't need authentication overhead.

## 5. JSON Files to SQLite/GORM for CLI State

Migrated CLI local storage from JSON files to SQLite via GORM.

**Before**: `~/.local/share/nebi/index.json` (workspace registry) + `~/.config/nebi/credentials.json` (auth tokens per server), manually serialized.

**After**: Single `~/.local/share/nebi/nebi.db` with GORM auto-migration, WAL mode, and tables: `workspaces`, `store_config` (singleton, ID=1), `store_credentials` (singleton, ID=1). Gets ACID guarantees and eliminates race conditions.

## 6. Shared Database Between CLI and Desktop

CLI and desktop app read/write the same `nebi.db` file using the same `store_config` and `store_credentials` tables.

Platform paths:
- Linux: `~/.local/share/nebi/nebi.db`
- macOS: `~/Library/Application Support/nebi/nebi.db`
- Windows: `%APPDATA%\nebi\nebi.db`

This means `nebi login` from CLI shows up in the desktop app and vice versa. The `RemoteServer` model was removed in favor of the shared singleton tables.

## 7. Remote Server Proxy (Local Mode Only)

Desktop app includes an API proxy (`/api/v1/remote/*`) so the frontend can browse a remote team server without the CLI. The handler uses `cliclient.Client` under the hood -- same HTTP client the CLI uses. Read-only: list workspaces, get versions/tags/manifests. Push/publish requires the CLI.

## 8. Shared Core Libraries Across CLI, REST Server, and Desktop

The CLI (`cmd/nebi/`), the REST API server (`internal/api/`), and the desktop app (`app.go`) all consume the same internal packages rather than having separate implementations:

- `internal/models/` -- shared data models (Workspace, Job, User, etc.)
- `internal/store/` -- GORM-based local state (config, credentials, workspace tracking). Used by CLI commands directly, and by the desktop REST server's remote handlers.
- `internal/cliclient/` -- HTTP client for talking to remote Nebi servers. Used by CLI commands (`nebi push`, `nebi pull`, etc.) and by the desktop's remote proxy handlers to forward requests.

This means a change to a model or store method is automatically picked up by all three consumers. Previously the CLI had its own JSON-based storage (`internal/localstore/`) separate from the server's GORM models, which led to drift (e.g., credentials stored in different tables).

## 9. Workspace Source Tracking

`Source` field on workspace model distinguishes `"local"` (created via `nebi init` in a directory) from `"managed"` (created via desktop UI/API). Helps the system understand workspace provenance.
