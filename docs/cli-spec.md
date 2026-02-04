# Nebi CLI Specification

## Command Overview

| Command | Description |
|---------|-------------|
| `nebi server list` | List configured servers (local + remotes) |
| `nebi server login <url>` | Login to remote server, make it active |
| `nebi server login local` | Switch to local server (no auth needed) |
| `nebi server logout` | Clear credentials, switch back to local |
| `nebi server status` | Show current server info (richer output for local) |
| `nebi registry add <name> <url>` | Add named registry |
| `nebi registry list` | List registries |
| `nebi registry remove <name>` | Remove registry |
| `nebi registry set-default <name>` | Set default registry (makes -r optional in most commands) |
| `nebi push <workspace>:<tag> [-r <registry>]` | Push pixi manifest to registry |
| `nebi pull <workspace>[:<tag>] [-r <registry>]` | Pull workspace from registry (default: latest) |
| `nebi pull <workspace>@<digest> [-r <registry>]` | Pull by digest (automation-friendly, immutable) |
| `nebi workspace list [-r <registry>]` | List workspaces (local or remote) |
| `nebi workspace tags <workspace> [-r <registry>]` | List tags for workspace |
| `nebi workspace info <workspace> [-r <registry>]` | Show workspace details (local or remote) |
| `nebi shell <workspace> [-r <registry>] -e <env>` | Activate env shell (uses pixi shell) |

## Common Flags

| Flag | Short | Used In | Description |
|------|-------|---------|-------------|
| `--registry` | `-r` | push, pull, workspace * | Named registry (optional if default set) |
| `--env` | `-e` | shell | Pixi environment name (e.g. pixi shell -e dev) |

---

## Registry Commands

```bash
# Add registries with credentials (depenent on the oci registry type for how to pass credentials in)
nebi registry add ds-team ghcr.io/myorg/data-science
nebi registry add infra-team ghcr.io/myorg/infra

# List all registries
nebi registry list

# Remove a registry
nebi registry remove ds-team

# Set default registry (makes -r optional on other commands)
nebi registry set-default ds-team
```

## Push/Pull Commands

```bash
# Push with tag
nebi push myenv:v1.0.0 -r ds-team

# Pull specific tag
nebi pull data-science:v1.0.0 -r ds-team

# Pull latest (default)
nebi pull data-science -r ds-team

# Pull by digest (automation-friendly, immutable)
nebi pull data-science@sha256:abc123def -r ds-team
```

## Workspace Commands (alias: ws)

```bash
# List local workspaces
nebi workspace list

# List remote workspaces from registry
nebi workspace list -r ds-team

# List tags for a workspace
nebi workspace tags data-science -r ds-team

# Create local env
use pixi commands directly

# Delete local env
use pixi commands directly

# Show workspace info
nebi workspace info myenv
nebi workspace info data-science -r ds-team

# Activate env shell (uses pixi shell)
nebi shell myenv
nebi shell data-science -r ds-team -e dev
```

## Server Commands

The CLI communicates with a Nebi server for all operations. By default, it uses a **local server** that auto-spawns on first use. Users can also connect to remote servers.

### Local Server (Default)

When `server: local` (the default), the CLI:
1. Checks for a running local server via `~/.local/share/nebi/server.state`
2. Auto-spawns one if not running (finds available port starting at 8460)
3. Authenticates using an auto-generated token from the state file (no login required)

**Files:**
- Config: `~/.config/nebi/config.yaml`
- State: `~/.local/share/nebi/server.state` (runtime: pid, port, token)
- Database: `~/.local/share/nebi/nebi.db` (SQLite)
- Logs: `~/.local/share/nebi/logs/server.log`

### Commands

```bash
# List all configured servers
nebi server list
#   NAME          URL                          STATUS
# * local         localhost:8463               running
#   company       https://nebi.company.com     authenticated

# Login to remote server (becomes active, prompts for credentials)
nebi server login https://nebi.company.com

# Switch back to local server (no auth needed)
nebi server login local

# Logout from current remote server, switch to local
nebi server logout

# Show current server status
nebi server status

# Local server output:
#   Server:     local
#   Status:     running
#   Port:       8463
#   PID:        12345
#   Uptime:     2h 34m
#   Logs:       ~/.local/share/nebi/logs/server.log
#   Database:   ~/.local/share/nebi/nebi.db

# Remote server output:
#   Server:     https://nebi.company.com
#   Status:     connected
#   User:       alice
#   Version:    1.2.0
```

### Config File

```yaml
# ~/.config/nebi/config.yaml
current_server: local

servers:
  local:
    # Special case - no URL needed, auto-spawns locally
  company:
    url: https://nebi.company.com
    token: eyJhbG...
```
