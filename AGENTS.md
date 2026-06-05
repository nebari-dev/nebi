# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## What Nebi is

Nebi is environment management for teams, built on top of [Pixi](https://pixi.sh). It versions, shares, and access-controls Pixi/uv environments, syncing them between machines and OCI registries (Quay, GHCR, etc.). It ships as a **single Go binary** that is simultaneously a CLI, an HTTP server, and (via Wails) a desktop app — all from one codebase.

## Commands

Run from the repo root unless noted.

```bash
# Dev (hot reload, frontend + backend together)
ADMIN_USERNAME=admin ADMIN_PASSWORD=<pw> make dev   # frontend :8461, backend :8460, docs :8460/docs

make install-tools   # installs swag, air, golangci-lint v1.64.8

# Build
make build           # full single binary: frontend build → embed → backend (bin/nebi)
make build-frontend  # just the React app, copies dist into internal/web/dist
make build-backend   # regenerates swagger, then go build (needs frontend/dist to exist)
make build-desktop   # Wails desktop app → build/bin/Nebi.app

# Lint / format (match CI)
make lint                          # go fmt + golangci-lint
cd frontend && npm run ci          # biome ci (frontend lint, what CI runs)
cd frontend && npm run check:fix   # biome autofix

# Tests
make test                                    # go test -tags=e2e -v ./...
go test -tags=e2e ./internal/service/...     # one package
go test -tags=e2e -run TestName ./cmd/nebi   # one test
make test-pkgmgr                             # pixi/uv package-manager tests
cd frontend && npm test                      # vitest run (single run)
cd frontend && npm run test:watch

make swagger         # regenerate API docs from serve.go annotations into internal/swagger
```

> **Build ordering matters.** The backend embeds `frontend/dist` via `go:embed`, so the frontend must be built (or stubbed with an empty `internal/web/dist`) before the Go binary will compile. `make build` handles this; CI builds the frontend as a separate job and downloads the artifact before the backend job.

> **Go tests require `-tags=e2e`.** End-to-end tests live behind that build tag and CI always passes it. A plain `go test ./...` skips them.

## Architecture

### One binary, two entry points
- `cmd/nebi/main.go` — the **CLI + server** binary. Uses cobra; subcommands (`init`, `push`, `pull`, `diff`, `serve`, `login`, …) are one file each in `cmd/nebi/`. `serve.go` boots the HTTP server.
- `main.go` + `app.go` (repo root, `package main`) — the **Wails desktop app**. It runs the same API router in-process on a goroutine, embedding `frontend/dist` directly. The desktop app always forces `NEBI_MODE=local`.

### Local mode vs. team mode
This is the single most important architectural distinction. `config.IsLocalMode()` (`NEBI_MODE`, default `team`) switches behavior throughout:
- **local** (desktop / single user): authentication is bypassed, casbin RBAC checks are skipped, all workspaces are visible, no encryption key needed.
- **team** (multi-user server): real auth (basic / JWT / OIDC-via-Keycloak), casbin RBAC enforcement, owner + permission/group-based workspace filtering, encrypted credentials.

When changing auth, visibility, or permissions, check both branches — see `internal/api/router.go` and the `isLocal` flags threaded through `internal/service`.

### Backend layers (`internal/`)
- `api/` — Gin router (`router.go`), `handlers/` (HTTP), `middleware/` (auth, RBAC, CORS, logging). The router wires together all dependencies based on mode.
- `service/` — business logic, the layer handlers call. Workspace lifecycle, publishing, permissions, groups, jobs.
- `db/` + `models/` — GORM models and migrations (SQLite by default; DSN via `NEBI_DATABASE_DSN`). `db.Migrate` runs server tables; `store.MigrateServerDB` adds local-mode store tables.
- `store/` — the **CLI-side** local index, config, and credentials (keyring). Distinct from the server's `db`; this is what the CLI reads/writes on the user's machine.
- `cliclient/` — HTTP client the CLI commands use to talk to a remote server (mirrors the handler endpoints).
- `auth/`, `rbac/` — authenticators (local/basic/OIDC) and casbin enforcer/provider.
- `queue/` (memory or valkey) + `worker/` + `executor/` (local or docker) — async job pipeline. Long operations (env builds, installs) are enqueued, run by the worker through an executor, with output streamed via `logstream/`.
- `pkgmgr/` — package-manager abstraction (`PackageManager` interface in `pkgmgr.go`, `pixi/` impl, selected by `factory.go`). This is where Pixi/uv commands are shelled out.
- `oci/` — push/pull of environments to OCI registries.
- `swagger/` — generated; do not hand-edit (run `make swagger`).

### Frontend (`frontend/`, embedded into the binary)
React 19 + TypeScript + Vite, **shadcn/ui + Tailwind v4**, tooled with **Biome** (not ESLint/Prettier) and **Vitest** (jsdom + MSW). Note state management here is **Zustand** (`src/store/`, e.g. `authStore`, `modeStore`) plus **TanStack Query** for server data — `src/api/*.ts` are the typed API clients (axios), one per backend resource. Pages in `src/pages/`, feature components grouped under `src/components/`. The `modeStore`/`viewModeStore` mirror the backend local-vs-team distinction in the UI.

In dev the Vite server (`:8461`) proxies to the backend (`:8460`); in production the built `dist` is served by the Go binary itself.

## Conventions
- Backend lint is `golangci-lint` (config in `.golangci.yml`); frontend lint/format is Biome (`frontend/biome.json`). CI runs `biome ci` and `go test -tags=e2e -race`.
- After changing API handler annotations, run `make swagger` so `internal/swagger` stays in sync.
- Local data dir defaults to `~/.local/share/nebi` (overridable with `NEBI_DATA_DIR`); the desktop app uses the OS app-data dir (see `getAppDataDir` in `app.go`).
