---
sidebar_position: 2
---

# CLI Overview

Nebi's CLI is organized into command groups: **Workspace**, **Sync**, **Connection**, and **Admin**.

## Workspace Commands

| Command | Description |
|---------|-------------|
| `nebi init` | Track current directory as a workspace (runs `pixi init` if needed) |
| `nebi status` | Show workspace sync status |
| `nebi workspace list` | List local and global workspaces |
| `nebi workspace promote <name>` | Copy current workspace to a global workspace |
| `nebi workspace remove <name>` | Remove a workspace from tracking |
| `nebi shell [name] [pixi-args...]` | Activate a pixi shell |
| `nebi run [name] [pixi-args...]` | Run a command or task via pixi |

## Sync Commands

| Command | Description |
|---------|-------------|
| `nebi push [<name>:]<tag>` | Push workspace specs to a server |
| `nebi pull [<name>[:<tag>]]` | Pull workspace specs from a server |
| `nebi diff [<ref-a>] [<ref-b>]` | Compare workspace specs |
| `nebi publish <name>:<tag>` | Publish to an OCI registry |
| `nebi import <oci-reference>` | Import a workspace from a public OCI registry |

## Connection Commands

| Command | Description |
|---------|-------------|
| `nebi login <server-url>` | Authenticate with a server |
| `nebi registry list` | List available OCI registries |
| `nebi registry create` | Create a new OCI registry |
| `nebi registry delete <name>` | Delete an OCI registry |

## Admin Commands

| Command | Description |
|---------|-------------|
| `nebi serve` | Run a Nebi server instance |

## Global Flags

- `-s, --server <name>` — Specify which server to use (defaults to the default server)
- `--version` — Print version information
