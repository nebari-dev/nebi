# Nebi CLI

Nebi is a command-line tool for managing and sharing Pixi workspaces via OCI registries.

## Commands

### Registry Management

| Command | Description |
|---------|-------------|
| `nebi registry add <name> <url>` | Add a named registry |
| `nebi registry list` | List configured registries |
| `nebi registry remove <name>` | Remove a registry |
| `nebi registry set-default <name>` | Set the default registry |

When a default registry is set, the `-r` flag becomes optional for most commands.

### Push & Pull

| Command | Description |
|---------|-------------|
| `nebi push <workspace>:<tag> [-r <registry>]` | Push workspace manifest to registry |
| `nebi pull <workspace>[:<tag>] [-r <registry>]` | Pull workspace from registry (default: latest) |
| `nebi pull <workspace>@<digest> [-r <registry>]` | Pull workspace by digest |

### Workspace Management

| Command | Description |
|---------|-------------|
| `nebi workspace list [-r <registry>]` | List workspaces (local or remote) |
| `nebi workspace tags <workspace> [-r <registry>]` | List tags for a workspace |
| `nebi workspace info <workspace> [-r <registry>]` | Show workspace details |
| `nebi shell <workspace> [-r <registry>] [-e <env>]` | Activate environment shell |

`workspace` can be aliased as `ws`.

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--registry` | `-r` | Named registry to use |
| `--env` | `-e` | Pixi environment name |
