---
sidebar_position: 2
---

# CLI Reference

Nebi's CLI is organized into command groups: **Workspace**, **Sync**, **Connection**, and **Admin**.

## Workspace Commands

| Command | Description |
|---------|-------------|
| `nebi init` | Track current directory as a workspace (runs `pixi init` if needed) |
| `nebi status` | Show workspace sync status |
| `nebi workspace list` | List tracked workspaces |
| `nebi workspace remove <name>` | Remove a workspace from tracking |
| `nebi workspace prune` | Remove workspaces whose paths no longer exist |
| `nebi shell [name] [pixi-args...]` | Activate a pixi shell |
| `nebi run [name] [pixi-args...]` | Run a command or task via pixi |

## Sync Commands

| Command | Description |
|---------|-------------|
| `nebi push [<name>][:<tag>]` | Push workspace specs to a server (tag optional, auto-tags with content hash + latest) |
| `nebi pull [<name>[:<tag>]]` | Pull workspace specs from a server |
| `nebi diff [<ref-a>] [<ref-b>]` | Compare workspace specs |
| `nebi publish [name]` | Publish to an OCI registry (uses content hash tag by default) |
| `nebi import <oci-reference>` | Import a workspace from a public OCI registry |

## Connection Commands

| Command | Description |
|---------|-------------|
| `nebi login <server-url>` | Authenticate with a server |
| `nebi registry list` | List available OCI registries |
| `nebi registry add` | Add an OCI registry |
| `nebi registry remove <name>` | Remove an OCI registry |

## Admin Commands

| Command | Description |
|---------|-------------|
| `nebi serve` | Run a Nebi server instance |

## Common Flags

- `-r, --remote` — Target the server instead of local workspaces (on `workspace list` and `workspace remove`)
- `--version` — Print version information
