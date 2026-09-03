# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## What Nebi is

Nebi is environment management for teams, built on top of [Pixi](https://pixi.sh). It versions, shares, and access-controls Pixi/uv environments, syncing them between machines and OCI registries (Quay, GHCR, etc.). It ships separate CLI, server, local web, and Wails desktop executables from one codebase.

## Commands

Run from the repo root unless noted.

```bash
# Dev (hot reload, frontend + backend together)
ADMIN_USERNAME=admin ADMIN_PASSWORD=<pw> make dev   # frontend :8461, backend :8460, docs :8460/docs

make install-tools   # installs swag, air, golangci-lint v1.64.8

# Build
make build           # CLI + server + local web binaries
make build-frontend  # just the React app, copies dist into internal/web/dist
make build-backend   # regenerates swagger, then go build (needs frontend/dist to exist)
make build-desktop   # Wails desktop app → build/bin/Nebi.app (executable: nebi-desktop)

# Lint / format (match CI)
make lint                          # go fmt + golangci-lint
cd frontend && npm run ci          # biome ci (frontend lint, what CI runs)
cd frontend && npm run check:fix   # biome autofix

# Tests
make test                                    # go test -tags=e2e -v ./...
go test -tags=e2e ./internal/service/...     # one package
go test -tags=e2e -run TestName ./cmd/nebi-cli   # one test
make test-pixi                               # pixi integration tests
cd frontend && npm test                      # vitest run (single run)
cd frontend && npm run test:watch

make swagger         # regenerate API docs from server annotations into internal/swagger
```

> **Build ordering matters.** The backend embeds `frontend/dist` via `go:embed`, so the frontend must be built (or stubbed with an empty `internal/web/dist`) before the Go binary will compile. `make build` handles this; CI builds the frontend as a separate job and downloads the artifact before the backend job.

> **Go tests require `-tags=e2e`.** End-to-end tests live behind that build tag and CI always passes it. A plain `go test ./...` skips them.

## Architecture

### Separate Binaries
- `cmd/nebi-cli/main.go` — the **CLI client** binary. Uses cobra; subcommands (`init`, `push`, `pull`, `diff`, `login`, …) are one file each in `cmd/nebi-cli/`.
- `cmd/nebi-server/main.go` — the **team server** binary. It boots the HTTP API server and worker in team mode.
- `cmd/nebi-web/main.go` — the **local web** binary. It boots the same embedded React frontend, API router, and worker in local mode.
- `main.go` + `app.go` (repo root, `package main`) — the **Wails desktop app**. It runs the same API router in-process on a goroutine, embedding `frontend/dist` directly.

### Local mode vs. team mode
This is the single most important architectural distinction. `config.IsLocalMode()` reflects the explicit runtime mode selected by the binary entry point:
- **local** (desktop / single user): authentication is bypassed, casbin RBAC checks are skipped, all workspaces are visible, no encryption key needed.
- **team** (multi-user server): real auth (basic / JWT / OIDC-via-Keycloak), casbin RBAC enforcement, owner + permission/group-based workspace filtering, encrypted credentials.

When changing auth, visibility, or permissions, check both branches — see `internal/api/router.go` and the `isLocal` flags threaded through `internal/service`.

### Backend layers (`internal/`)
- `api/` — Gin router (`router.go`), `handlers/` (HTTP), `middleware/` (auth, RBAC, CORS, logging). The router wires together all dependencies based on mode.
- `service/` — business logic, the layer handlers call. Workspace lifecycle, publishing, permissions, groups, jobs.
- `db/` + `models/` — GORM models and migrations (SQLite by default; DSN via `NEBI_DATABASE_DSN`). `db.Migrate` runs server tables; `store.MigrateServerDB` adds local-mode store tables.
- `store/` — the **CLI-side** local index, config, and credentials (keyring). Distinct from the server's `db`; this is what the CLI reads/writes on the user's machine.
- `cliclient/` — HTTP client the CLI commands use to talk to a remote server (mirrors the handler endpoints).
- `auth/`, `rbac/` — authenticators (local/basic/OIDC) and casbin enforcer/provider. Every externally-authenticated login path must resolve users through `auth.findOrCreateFederatedUser`, matching existing external identities only by `(issuer, subject)` and never by mutable username/email claims. If that flow returns a review-gated error, clients should check `auth.FederatedIdentityReviewErrorCode` before falling through to a generic 401.
- `queue/` (memory or valkey) + `worker/` + `executor/` (local or docker) — async job pipeline. Long operations (env builds, installs) are enqueued, run by the worker through an executor, with output streamed via `logstream/`.
- `pixi/` — pixi integration (`PixiManager`). This is where pixi commands are shelled out.
- `oci/` — push/pull of environments to OCI registries.
- `swagger/` — generated; do not hand-edit (run `make swagger`).

### Frontend (`frontend/`, embedded into the binary)
React 19 + TypeScript + Vite, **shadcn/ui + Tailwind v4**, tooled with **Biome** (not ESLint/Prettier) and **Vitest** (jsdom + MSW). Note state management here is **Zustand** (`src/store/`, e.g. `authStore`, `modeStore`) plus **TanStack Query** for server data — `src/api/*.ts` are the typed API clients (axios), one per backend resource. Pages in `src/pages/`, feature components grouped under `src/components/`. The `modeStore`/`viewModeStore` mirror the backend local-vs-team distinction in the UI.

In dev the Vite server (`:8461`) proxies to the backend (`:8460`); in production the built `dist` is served by the Go binary itself.

Two frontend invariants worth knowing (see issue #217):
- **TanStack Query `networkMode` is per app mode**, set in `src/lib/queryClient.ts` + `src/store/modeStore.ts`: `'always'` in local (desktop) mode because the loopback backend stays reachable even when the OS reports offline, `'online'` in team mode where the API is a real network hop. Every query and mutation inherits this default — don't pin `'online'` in a code path that can run in the desktop app, or offline events will wedge loopback queries in `paused`.
- **`GET /remote/server` reporting `status: 'connected'` means a server URL + token are stored** (a local DB read), not that the remote is reachable. Reachability surfaces as errors on the remote data queries; pages gate remote data and the unreachable banner with `useRemoteView()` from `src/hooks/useRemote.ts`, and consume the `isFirstLoad` / `isUnreachable` flags the remote query hooks return rather than re-deriving them from TanStack internals. Any query whose `isUnreachable` flag a page renders must keep retrying on its own — `pollWithErrorBackoff` if it polls anyway, `retryWhileUnreachable` if it shouldn't poll while healthy — because an errored query without an interval only refetches on remount (`refetchOnWindowFocus` is `false` app-wide, so focus is not a recovery path), so the banner would stick after the server recovered. Queries wrapped in `withRemoteFlags` must also pin `notifyOnChangeProps` (the wrapper spreads the result, which otherwise marks every field tracked and re-renders consumers on every poll tick). The remote workspace *detail* page (`RemoteWorkspaceDetail.tsx`) and the per-workspace hooks predate this pattern and are not yet migrated (tracked in https://github.com/nebari-dev/nebi/issues/507). If that status ever becomes a real liveness probe, revisit the banner logic, which assumes it never flips on remote outages.

## Conventions
- Backend lint is `golangci-lint` (config in `.golangci.yml`); frontend lint/format is Biome (`frontend/biome.json`). CI runs `biome ci` and `go test -tags=e2e -race`.
- After changing API handler annotations, run `make swagger` so `internal/swagger` stays in sync.
- Local data dir defaults to `~/.local/share/nebi` (overridable with `NEBI_DATA_DIR`); the desktop app uses the OS app-data dir (see `getAppDataDir` in `app.go`).
