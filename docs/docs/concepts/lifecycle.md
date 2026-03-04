---
sidebar_position: 2
---

# Lifecycle: Local, Server, Published

Nebi workspaces exist in three stages. You only use the ones you need — most users start local and add server syncing when they're ready to collaborate.

## The Three Stages

```
Local (your machine) → Server (your team) → Published (the world)
```

### Local

A local workspace is a Pixi project tracked by Nebi on your machine. This is the starting point for everything.

**What you can do locally (no server needed):**

- Track workspaces with `nebi init`
- List and manage workspaces with `nebi workspace list`
- Activate environments by name with `nebi shell` and `nebi run`
- Import published environments with `nebi import`

Local workspaces are stored as entries in Nebi's local SQLite database. The actual environment files (`pixi.toml`, `pixi.lock`) live in your project directory as normal.

### Server

A server workspace is a versioned copy of your `pixi.toml` and `pixi.lock` stored on a shared Nebi server. This is how you share environments with your team.

**What the server adds:**

- Push and pull workspace specs with `nebi push` and `nebi pull`
- Browse team workspaces with `nebi workspace list --remote`
- Version history with content-addressed tags
- Access control (who can read/write each workspace)
- Diff between local and server versions with `nebi diff`

The server stores *specs only* — not installed packages. When someone pulls a workspace, they get the `pixi.toml` and `pixi.lock` files and run `pixi install` locally.

### Published

A published workspace is an OCI artifact stored in a container registry (GitHub Container Registry, Quay.io, etc.). This is for distributing environments beyond your team.

**What publishing adds:**

- Broad distribution without requiring access to your Nebi server
- Immutable references via OCI digests
- Compatibility with any OCI-compliant registry
- Import without authentication (for public registries)

Publishing is a server-side operation — you push to the server first, then publish from there to the registry.

## How They Connect

The typical progression is:

1. **Create locally:** `nebi init` + edit `pixi.toml` with Pixi
2. **Share with team:** `nebi push` to server, teammates `nebi pull`
3. **Distribute broadly:** `nebi publish` to OCI registry, anyone can `nebi import`

You can skip steps. For example:

- **Local only:** Just use `nebi init`, `nebi shell`, and `nebi run` without ever connecting to a server.
- **Import directly:** Use `nebi import` to pull from a public OCI registry without having your own server.

## What Moves Between Stages

Only two files move between stages: `pixi.toml` and `pixi.lock`. Nebi never syncs installed packages, source code, or data files. Each machine resolves the locked dependencies locally with `pixi install`.
