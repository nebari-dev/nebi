# Nebi components

It has three components: a **CLI**, a **desktop app**, and a **team server**. All share the same core libraries but serve different use cases.

## System overview

<img src="/img/context-diagram.svg" alt="Nebi System Context diagram" style={{maxWidth: '100%', height: 'auto'}} />

## CLI

The CLI is a standalone tool for managing and tracking Pixi workspaces on your local machines, both for [solo use](./cli-local.md) and for [team workflows](./cli-team.md) backed by a Nebi server. Every command is documented in the [CLI reference](./cli-reference.md).

- **Local database**: Track workspace names, paths, and versions in a local database
- **Pixi shell/run**: Open a pixi shell or run pixi tasks by workspace name
- **Publish/import - OCI registries**: Push workspace specs directly to OCI registries, and import specs from published registries
- **Push/pull - Nebi server**: Sync (push) versioned `pixi.toml` and `pixi.lock` specs to a Nebi server, and pull from a different machine
- **Track state (diff)**: Compare specs between local directories, workspace names, or server versions

## Desktop application

The [desktop app](./desktop.md) is another tool for managing Pixi workspaces on your local machines. It supports all the CLI workflows through a graphical / app interface.

## OCI Registries

Nebi can publish workspace specifications (`pixi.toml` & `pixi.lock`) as OCI artifacts to any [OCI-compliant registry](./registry-setup.md) such as GitHub Container Registry, Quay.io, or self-hosted registries.

- Publishing can be done from the CLI (`nebi publish`) or triggered from the desktop app or server
- The desktop app and server UI includes a registry browser for discovering and pulling published workspaces

Specs are packed into an OCI Image Manifest with custom media types (`application/vnd.pixi.toml.v1+toml`, `application/vnd.pixi.lock.v1+yaml`). Each push creates a content-addressed tag (`sha-<hash>`) plus a `latest` tag and any user-specified tags.

## Nebi Server

The [Nebi server](./server-setup.md) is the team deployment of Nebi. It runs the desktop app interface but in a **server and team mode** with full multi-user support.

- **Authentication**: JWT-based sessions with pluggable backends — basic auth, OIDC, or proxy auth
- **Role-based AC**: Apache Casbin-based access control with per-workspace permissions (read, write, admin) for users
- **API**: Git-based HTTP server serving the REST API and the bundled React frontend
- **Central database**: SQLite (default) or PostgreSQL for workspace and user tracking
- **Background worker**: Background processor for async operations like workspace creation and package installation
- **Job queue**: In-memory queue (single instance) or Valkey (distributed deployments) to process workspace creation and updating requests.

Learn more: [Nebi server setup](./server-setup.md).
