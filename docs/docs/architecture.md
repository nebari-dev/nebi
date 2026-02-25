---
sidebar_position: 2
---

# Architecture

Nebi is built as a single Go codebase with a React frontend. It runs in three forms: a **desktop app**, a **CLI**, and a **team server**. All three share the same core libraries but serve different use cases.

## System Overview

<img src="/img/architecture.svg" alt="Nebi architecture diagram" style={{maxWidth: '100%', height: 'auto'}} />

## Desktop App

The desktop app is built with [Wails](https://wails.io/), which embeds the Go backend and React frontend into a native window. It runs in **local mode** — no authentication, single user.

- The embedded server starts on port 8460
- Data is stored in platform-standard directories (`~/.local/share/nebi/` on Linux, `~/Library/Application Support/nebi/` on macOS)
- The React UI communicates with the Go backend over HTTP, same as the server deployment

Build with `wails build` (or `wails dev` for development with hot reload).

## Nebi Server

The server is the team deployment of Nebi. It runs the same Go backend as the desktop app but in **team mode** with full multi-user support.

- **API**: Gin-based HTTP server serving the REST API and the bundled React frontend
- **Auth**: JWT-based sessions with pluggable backends — basic auth, OIDC, or proxy auth
- **RBAC**: Casbin-based access control with per-workspace permissions (read, write, admin)
- **Database**: SQLite (default) or PostgreSQL
- **Job Queue**: In-memory queue (single instance) or Valkey (distributed deployments)
- **Worker**: Background processor for async operations like workspace creation and package installation

Start with `nebi serve`. Configure via environment variables (`NEBI_*` prefix) or `config.yaml`.

## CLI

The CLI is a local-first tool for managing Pixi workspaces. It works standalone — no server required for basic workspace tracking.

- **Local store**: SQLite database tracking workspace names, paths, and sync state
- **Push/pull**: Sync versioned `pixi.toml` and `pixi.lock` specs to a Nebi server with content-addressed deduplication
- **Diff**: Compare specs between local directories, workspace names, or server versions
- **Shell/run**: Open a pixi shell or run pixi tasks by workspace name
- **Publish**: Push workspace specs directly to OCI registries

The CLI connects to a single configured server. Run `nebi login <url>` to set it up.

## OCI Registries

Nebi can publish workspace specs (`pixi.toml` + `pixi.lock`) as OCI artifacts to any OCI-compliant registry — GitHub Container Registry, Quay.io, or self-hosted registries.

- Specs are packed into an OCI Image Manifest with custom media types (`application/vnd.pixi.toml.v1+toml`, `application/vnd.pixi.lock.v1+yaml`)
- Each push creates a content-addressed tag (`sha-<hash>`) plus a `latest` tag and any user-specified tags
- Publishing can be done from the CLI (`nebi publish`) or triggered from the server
- The server UI includes a registry browser for discovering and pulling published workspaces

## Key Workflows

**Push/Pull** — The CLI reads the local `pixi.toml` and `pixi.lock`, sends them to the server as a new workspace version, and tags the version with a content hash plus any user-provided tag. Pulling downloads a tagged version and writes the files locally.

**Workspace Creation (Server)** — A workspace create request is enqueued as a job. The worker picks it up, runs `pixi init` or applies the provided spec via the executor, and streams logs back to the UI in real time.

**Publish to OCI** — The CLI or server packs the workspace spec into an OCI manifest and pushes it to the configured registry. Other users can browse the registry from the Nebi UI and pull specs back into their workspaces.
