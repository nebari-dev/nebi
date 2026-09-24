---
title: "CLI Reference"
---

Nebi's CLI is `nebi`. It is organized into command groups: **Project**, **Sync**, and **Connection**.

## Specs vs. Bundles

Two terms appear throughout these commands:

- **Project spec**: `pixi.toml` and `pixi.lock`. The minimal environment definition. Stored on the Nebi server.
- **Project bundle**: `pixi.toml` + `pixi.lock` + any project files (READMEs, source code, data), packaged for an OCI registry.

## Project Commands

| Command | Description |
|---------|-------------|
| `nebi init` | Track current directory as a project (runs `pixi init` if needed) |
| `nebi status` | Show project sync status |
| `nebi project list` | List tracked projects |
| `nebi project install <name>` | Install a server project's environment from its lockfile (local mode) |
| `nebi project uninstall <name>` | Remove a server project's installed environment (local mode) |
| `nebi project remove <name>` | Remove a project from tracking |
| `nebi project prune` | Remove projects whose paths no longer exist |
| `nebi shell [name] [pixi-args...]` | Activate a pixi shell |
| `nebi run [name] [pixi-args...]` | Run a command or task via pixi |

## Sync Commands

| Command | Description |
|---------|-------------|
| `nebi push [<name>][:<tag>]` | Push project specs to a server (tag optional, auto-tags with content hash + latest) |
| `nebi pull [<name>[:<tag>]]` | Pull project specs from a server |
| `nebi diff [<ref-a>] [<ref-b>]` | Compare project specs |
| `nebi publish [name]` | Publish a project bundle to an OCI registry (uses content hash tag by default) |
| `nebi import <oci-reference>` | Import a project bundle from an OCI registry, restoring pixi files and asset layers |

## Connection Commands

| Command | Description |
|---------|-------------|
| `nebi login <server-url>` | Authenticate with a server |
| `nebi registry list` | List available OCI registries |
| `nebi registry add` | Add an OCI registry |
| `nebi registry remove <name>` | Remove an OCI registry |

## Flags

**`publish`**

- `--local`: Publish directly to registry without a server
- `--tag <tag>`: Set the OCI tag (default: content hash with `--local`, auto-incrementing `v1`, `v2`, ... otherwise)
- `--repo <name>`: Set the OCI repository name (defaults to the project name)
- `--registry <name>`: Registry name or ID to publish to (defaults to the configured default registry)
- `--concurrency N`: Number of files uploaded at the same time (only with `--local`, default 8)

**`import`**

- `-o, --output <dir>`: Output directory (defaults to current directory)
- `--concurrency N`: Number of files downloaded at the same time (default 8)
- `--force`: Overwrite an existing `pixi.toml` without asking. Only applies when the bundle contains just pixi files; bundles with other files always refuse to overwrite.

**`project list`, `project remove`**

- `-r, --remote`: Use projects from the Nebi server instead of local projects
- `--installed` (`project list` only): Only list server projects with an installed environment; implies `--remote`
