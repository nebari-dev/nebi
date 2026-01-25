# Nebi CLI Specification (v2) - Proposed

> This document consolidates all CLI commands from the original `cli-spec.md`, `design/duplicate-pulls.md`,
> and `design/diff-workflow.md` into a single reference.

## Refinements from v1

The following conventions were standardized across the CLI:

1. **`-C` for path override** — Matches git/cargo/make convention. The long form is `--directory` (not `--path`) to match git semantics ("run as if invoked from this directory").
2. **`--name` / `-n` for aliases** — Short flag added for convenience on global workspaces.
3. **`-f` short flag for `--force`** — Standard POSIX convention.
4. **`--dry-run` / `-n` conflict avoided** — `--dry-run` has no short form (it's infrequent enough). `-n` is reserved for `--name`.
5. **`nebi ws` alias** — Documented as a first-class short alias for `nebi workspace` (like `kubectl` → `k` conventions, but built-in).
6. **`--force` vs `--yes`** — Clarified: `--yes`/`-y` skips interactive prompts (safe operations). `--force`/`-f` overrides safety checks (destructive operations like overwriting tags or re-pulling over modifications).
7. **`nebi workspace tags`** — Promoted from `nebi workspace list tags` (awkward nested subcommand) to `nebi workspace tags <workspace>`.
8. **Top-level `status`/`diff`** — These are high-frequency commands (like `git status`/`git diff`). No `nebi workspace` prefix required. `nebi workspace info` is kept as an alias for discoverability.
9. **`nebi status` accepts optional positional ref** — `nebi status data-science:v1.0` works from anywhere (uses index lookup). This mirrors `nebi shell` which already accepts a workspace ref. `-C` is kept as an alternative for explicit directory targeting.

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
| `nebi push --dry-run` | Preview what would be pushed (shows diff) |
| `nebi pull <workspace>[:<tag>] [-r <registry>]` | Pull workspace from registry (default: latest) |
| `nebi pull <workspace>@<digest> [-r <registry>]` | Pull by digest (automation-friendly, immutable) |
| `nebi pull --global <workspace>:<tag>` | Pull to global storage (one copy per workspace:tag) |
| `nebi workspace list [-r <registry>]` | List workspaces (local or remote) |
| `nebi workspace list --local` | List local workspaces with drift status |
| `nebi workspace tags <workspace> [-r <registry>]` | List tags for workspace |
| `nebi workspace info <workspace> [-r <registry>]` | Show workspace details (local or remote) |
| `nebi workspace prune` | Remove stale entries from the local index |
| `nebi shell [<workspace>] [-r <registry>] [-e <env>]` | Activate env shell (uses pixi shell) |
| `nebi status [<workspace>] [--remote] [-v]` | Show workspace drift status (default: cwd) |
| `nebi diff [source] [target] [--remote]` | Show detailed differences between workspace versions |

> **Aliases**: `nebi ws` = `nebi workspace`. `nebi status` = `nebi ws info` (in current directory).

---

## Common Flags

| Flag | Short | Used In | Description |
|------|-------|---------|-------------|
| `--registry` | `-r` | push, pull, workspace *, diff | Named registry (optional if default set) |
| `--env` | `-e` | shell | Pixi environment name (e.g. `pixi shell -e dev`) |
| `--global` | `-g` | pull | Pull to global storage instead of current directory |
| `--force` | `-f` | pull, push | Override safety checks (overwrite tags, re-pull over modifications) |
| `--yes` | `-y` | pull | Skip interactive prompts (auto-confirm, e.g. for CI) |
| `--remote` | | status, diff | Check/compare against current remote registry state |
| `--json` | | status, diff, workspace list | Output as JSON for scripting |
| `--verbose` | `-v` | status | Verbose output (full digests, next-step suggestions) |
| `--dry-run` | | push | Preview what would be pushed without pushing |
| `--lock` | | diff | Show full lock file diff (default: summary only) |
| `--toml` | | diff | Show only pixi.toml diff |
| `--directory` | `-C` | status, diff, shell | Target workspace by directory path (like `git -C`). Alternative to positional ref. |
| `--name` | `-n` | pull --global, shell | Human-friendly alias for global workspace |

---

## Registry Commands

```bash
# Add registries with credentials (dependent on the OCI registry type)
nebi registry add ds-team ghcr.io/myorg/data-science
nebi registry add infra-team ghcr.io/myorg/infra

# List all registries
nebi registry list

# Remove a registry
nebi registry remove ds-team

# Set default registry (makes -r optional on other commands)
nebi registry set-default ds-team
```

---

## Push/Pull Commands

### Push

```bash
# Push with tag
nebi push myenv:v1.0.0 -r ds-team

# Push with dry-run (preview what would be published)
nebi push data-science:v1.1 --dry-run

# Push modified workspace (notes origin, suggests new tag)
cd ~/project-a && nebi push data-science:v1.1
```

**Push conflict behavior:**
- Pushing to a tag with different existing content is blocked by default
- Use `-f` / `--force` to overwrite an existing tag (may break other users)
- The CLI will suggest pushing as a new tag instead

### Pull (Directory - Default)

Directory pulls write to the current directory. Duplicates to different directories are always allowed.

```bash
# Pull specific tag
nebi pull data-science:v1.0.0 -r ds-team

# Pull latest (default)
nebi pull data-science -r ds-team

# Pull by digest (automation-friendly, immutable)
nebi pull data-science@sha256:abc123def -r ds-team

# Re-pull to same directory (overwrites, updates index)
nebi pull data-science:v1.0 -r ds-team

# Pull different tag to same directory (prompts to confirm overwrite)
nebi pull data-science:v2.0 -r ds-team

# Force overwrite without prompt
nebi pull data-science:v1.0 -f

# Non-interactive mode (for CI — confirms prompts, doesn't override safety)
nebi pull data-science:v1.0 -y
```

### Pull (Global)

Global pulls write to `~/.local/share/nebi/workspaces/`. Only one copy per `workspace:tag` is allowed globally.

```bash
# First global pull
nebi pull --global data-science:v1.0 -r ds-team

# Duplicate global pull (blocked by default)
nebi pull --global data-science:v1.0 -r ds-team
# Error: data-science:v1.0 already exists globally. Use --force to re-pull.

# Force re-pull globally
nebi pull -g data-science:v1.0 -r ds-team -f

# Global pull with alias
nebi pull -g data-science:v1.0 -n ds-stable

# Different tag (separate directory, allowed)
nebi pull --global data-science:v2.0 -r ds-team
```

### `--force` vs `--yes`

These flags serve different purposes:

| Flag | Short | Meaning | Example |
|------|-------|---------|---------|
| `--yes` | `-y` | "Skip prompts, use defaults" | Pulling same tag to new dir: auto-confirms |
| `--force` | `-f` | "Override safety checks" | Overwriting a different tag, re-pulling globally |

In CI, you'll typically want `-y` (don't prompt me) but rarely `-f` (override protections).

### Pull Behavior Summary

| Scenario | Behavior |
|----------|----------|
| Directory pull (new location) | Always succeeds, adds to index |
| Directory pull (same location, same tag) | Overwrites, updates index entry |
| Directory pull (different tag, same location) | Prompts to confirm overwrite |
| Global pull (new workspace:tag) | Succeeds, adds to index |
| Global pull (existing workspace:tag) | Blocked, suggests `-f` |
| Global pull (different tag) | Succeeds (separate directory) |

---

## Workspace Commands (alias: `ws`)

`nebi workspace` can be shortened to `nebi ws` in all commands below.

```bash
# List local workspaces (with drift status)
nebi ws list --local

# List remote workspaces from registry
nebi ws list -r ds-team

# List tags for a workspace
nebi ws tags data-science -r ds-team

# Show workspace info (alias for nebi status when given a name)
nebi ws info myenv
nebi ws info data-science -r ds-team

# Remove stale entries from local index
nebi ws prune
```

### Local Workspace List Output

```
$ nebi workspace list --local
LOCAL WORKSPACES
  WORKSPACE           TAG       STATUS      LOCATION
  data-science        v1.0      modified    ~/project-a (local)
  data-science        v1.0      clean       ~/project-b (local)
  data-science        v2.0      clean       ~/.local/share/nebi/... (global)
  ml-tools            latest    clean       ~/ml-project (local)
```

**Status values:**
- `clean`: Local file hashes match the pulled layer digests
- `modified`: Local file hashes differ from pulled layer digests
- `missing`: Path no longer exists
- `unknown`: Cannot determine (e.g., `.nebi` metadata missing)

---

## Shell Command

```bash
# Activate from workspace directory (uses local copy)
cd ~/project-a && nebi shell

# Activate by name (prefers global -> most recent local -> prompts if multiple)
nebi shell data-science:v1.0

# Activate with specific pixi environment
nebi shell data-science -r ds-team -e dev

# Activate global workspace by alias
nebi shell -n ds-stable

# Activate workspace at specific directory
nebi shell -C ~/project-a
```

**Shell activation priority:**
1. If in a workspace directory (has `.nebi`), uses local copy
2. Prefers global copy if available
3. Falls back to most recent local copy
4. Prompts if multiple local copies exist (no global)

**Drift warning:** If the workspace has been modified since pull, shell warns but proceeds:
```
$ nebi shell data-science:v1.0
⚠ Local copy at ~/project-a has been modified since pull.
  Origin: data-science:v1.0 (sha256:abc123...)
Activating modified local copy...
(data-science) $
```

---

## Status Command

Show the current state of a local workspace. Checks for drift against what was originally pulled.

```
nebi status [<workspace>[:<tag>]] [--remote] [--json] [-v|--verbose] [-C <dir>]
```

### Arguments

| Argument | Description |
|----------|-------------|
| `<workspace>[:<tag>]` | Optional. Workspace to check (uses index lookup + disambiguation). If omitted, uses current directory. |

### Flags

| Flag | Description |
|------|-------------|
| `--remote` | Also check remote registry for tag updates |
| `--json` | Output as JSON for scripting |
| `-v`, `--verbose` | Full digests, per-file status, next-step suggestions |
| `-C`, `--directory` | Operate on workspace at specified directory (overrides positional arg) |

### Targeting a workspace

Three ways to specify which workspace to check (mirrors `nebi shell` behavior):

```bash
# 1. Current directory (default) — looks for .nebi in cwd
nebi status

# 2. By workspace ref — looks up in local index
nebi status data-science:v1.0

# 3. By directory path — checks that specific location
nebi status -C ~/project-a
```

When using a workspace ref and multiple local copies exist, the same disambiguation
logic as `nebi shell` applies (prefers global → most recent local → prompts).

### Examples

**Compact (default):**
```
$ nebi status
data-science:v1.0 (ds-team)  •  pulled 7 days ago  •  modified

# Or by ref, from anywhere:
$ nebi status data-science:v1.0
data-science:v1.0 (ds-team)  •  ~/project-a  •  pulled 7 days ago  •  modified
```

**Verbose:**
```
$ nebi status -v
Workspace: data-science:v1.0
Registry:  ds-team
Pulled:    2025-01-15 10:30:00 (7 days ago)
Digest:    sha256:abc123def456...

Status:    modified
  pixi.toml:  modified
  pixi.lock:  modified

Next steps:
  nebi diff           # See what changed
  nebi pull -f        # Discard local changes
  nebi push :v1.1     # Publish as new version
```

**With remote check:**
```
$ nebi status --remote
Workspace: data-science:v1.0 (from ds-team registry)
Pulled:        2025-01-15 (7 days ago)
Origin digest: sha256:abc123...

Local status: modified (pixi.toml, pixi.lock)

Remote status:
  ⚠ Tag 'v1.0' now points to sha256:xyz789... (was sha256:abc123... when pulled)
  The tag has been updated since you pulled.

Run 'nebi diff' to see your local changes (vs what you pulled).
Run 'nebi diff --remote' to see diff against current remote.
Run 'nebi pull -f' to update local copy (overwrites local changes).
```

**JSON output:**
```json
{
  "workspace": "data-science",
  "tag": "v1.0",
  "registry": "ds-team",
  "pulled_at": "2025-01-15T10:30:00Z",
  "origin_digest": "sha256:abc123...",
  "local": {
    "pixi_toml": "modified",
    "pixi_lock": "modified"
  },
  "remote": {
    "current_tag_digest": "sha256:xyz789...",
    "tag_has_moved": true,
    "origin_still_exists": true
  }
}
```

### Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Clean / no differences |
| `1` | Differences detected (local drift or remote changes) |
| `2` | Error (invalid args, network error, parse failure) |

---

## Diff Command

Show detailed differences between workspace versions. While `nebi status` answers "has anything changed?", `nebi diff` answers "what exactly changed?".

```
nebi diff [source] [target] [--remote] [--json] [--lock] [--toml] [-C <dir>]
```

### Arguments

| Argument | Description |
|----------|-------------|
| `source` | Source reference (default: pulled version from `.nebi`) |
| `target` | Target reference (default: local files). Can be a directory path or `workspace:tag`. |

### Flags

| Flag | Description |
|------|-------------|
| `--remote` | Compare local against current remote tag content |
| `--json` | Output as JSON for scripting |
| `--lock` | Show full lock file diff (default: summary only) |
| `--toml` | Show only pixi.toml diff |
| `-C`, `--directory` | Operate on workspace at specified directory |

### Usage Patterns

```bash
# Local changes vs what was pulled (uses .nebi digests, fetches origin by digest)
nebi diff

# Local vs current remote tag (fetches by tag, may differ if tag was overwritten)
nebi diff --remote

# Between two registry versions
nebi diff data-science:v1.0 data-science:v2.0

# Between registry version and local
nebi diff data-science:v1.0 .
```

### Output (Unified Diff)

```
$ nebi diff
--- pulled (data-science:v1.0, sha256:abc123...)
+++ local
@@ pixi.toml @@
 [workspace]
-version = "0.1.0"
+version = "0.2.0"

 [dependencies]
-numpy = ">=2.0.0,<3"
+numpy = ">=2.4.1,<3"
+scipy = ">=1.17.0,<2"
-old-package = "*"

@@ pixi.lock (summary) @@
 12 packages changed:
   Added (3):    scipy, liblapack, libopenblas
   Removed (1):  old-package
   Updated (8):  numpy (2.0.0 -> 2.4.1), python (3.11 -> 3.12), ...

[Use --lock for full lock file diff]
```

### Remote Diff

```
$ nebi diff --remote
Note: Tag 'v1.0' has been updated since you pulled (was sha256:aaa...).
      Showing diff against current remote content.

--- remote (data-science:v1.0, current: sha256:bbb...)
+++ local
@@ pixi.toml @@
 [dependencies]
+scipy = ">=1.17.0,<2"
-pandas = ">=2.0.0,<3"
 ...
```

### Between Registry Versions

```
$ nebi diff data-science:v1.0 data-science:v2.0
--- data-science:v1.0
+++ data-science:v2.0
@@ pixi.toml @@
 [dependencies]
-python = ">=3.11"
+python = ">=3.12"
-tensorflow = ">=2.14.0"
+torch = ">=2.0.0,<3"
+transformers = ">=4.35.0"

@@ pixi.lock (summary) @@
 45 packages added, 12 removed, 8 updated
```

### JSON Output

```json
{
  "source": {
    "type": "pulled",
    "workspace": "data-science",
    "tag": "v1.0",
    "digest": "sha256:abc123..."
  },
  "target": {
    "type": "local",
    "path": "/home/user/my-project"
  },
  "pixi_toml": {
    "added": [
      {"section": "dependencies", "key": "scipy", "value": ">=1.17.0,<2"}
    ],
    "removed": [
      {"section": "dependencies", "key": "tensorflow", "value": ">=2.14.0"}
    ],
    "modified": [
      {"section": "dependencies", "key": "numpy", "old": ">=2.0.0,<3", "new": ">=2.4.1,<3"}
    ]
  },
  "pixi_lock": {
    "packages_added": 3,
    "packages_removed": 1,
    "packages_updated": 8,
    "added": ["scipy 1.17.0", "liblapack 3.11.0", "libopenblas 0.3.30"],
    "removed": ["old-package 1.0.0"],
    "updated": [
      {"name": "numpy", "old": "2.0.0", "new": "2.4.1"},
      {"name": "python", "old": "3.11.0", "new": "3.12.0"}
    ]
  }
}
```

### What Each Diff Command Compares

| Command | Source | Target | Notes |
|---------|--------|--------|-------|
| `nebi diff` | Origin (by digest) | Local files | Always correct - uses immutable digest |
| `nebi diff --remote` | Current tag (by tag) | Local files | Shows what's different from *current* registry state |
| `nebi diff ref1 ref2` | ref1 (by tag) | ref2 (by tag) | Between two registry versions |
| `nebi diff ref .` | ref (by tag) | Local files | Registry version vs local |

### Exit Codes

| Code | Meaning |
|------|---------|
| `0` | No differences |
| `1` | Differences detected |
| `2` | Error (invalid args, network error, parse failure) |

---

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

---

## Local Storage & Metadata

### Local Index

Stored at `~/.local/share/nebi/index.json`. Tracks all pulled workspaces (both directory and global).

```jsonc
{
  "version": 1,
  "workspaces": [
    {
      "workspace": "data-science",
      "tag": "v1.0",
      "registry": "ds-team",
      "registry_url": "ghcr.io/myorg",
      "source": "oci",
      "path": "/home/user/project-a",
      "is_global": false,
      "pulled_at": "2024-01-20T10:30:00Z",
      "manifest_digest": "sha256:abc123def456...",
      "layers": {
        "pixi.toml": "sha256:111...",
        "pixi.lock": "sha256:222..."
      }
    }
  ],
  "aliases": {
    "ds-stable": {"uuid": "550e8400-...", "tag": "v1.0"}
  }
}
```

### `.nebi` Metadata File

Written to each pulled workspace directory for drift detection and origin tracking:

```yaml
# ~/project-a/.nebi
origin:
  workspace: data-science
  tag: v1.0
  registry: ds-team
  registry_url: ghcr.io/myorg
  manifest_digest: sha256:abc123def456...
  pulled_at: 2024-01-20T10:30:00Z
  source: oci                            # "oci" or "server"
  server_url: https://nebi.example.com   # Only if source: server
  server_version_id: 42                  # Only if source: server

layers:
  pixi.toml:
    digest: sha256:111...
    size: 2345
    media_type: application/vnd.pixi.toml.v1+toml
  pixi.lock:
    digest: sha256:222...
    size: 45678
    media_type: application/vnd.pixi.lock.v1+yaml
```

### Global Workspace Storage

```
~/.local/share/nebi/
├── index.json
└── workspaces/
    └── <uuid>/
        ├── v1.0/
        │   ├── pixi.toml
        │   ├── pixi.lock
        │   └── .nebi
        └── v2.0/
            ├── pixi.toml
            ├── pixi.lock
            └── .nebi
```

---

## Error Handling

| Scenario | Behavior |
|----------|----------|
| No `.nebi` file (for status/diff) | Error: "Not a nebi workspace. Run 'nebi pull' first." |
| `.nebi` file corrupted | Error: "Invalid .nebi metadata. Re-pull to fix." |
| Remote unreachable (`--remote`) | Warning: "Cannot reach registry. Showing local status only." |
| `pixi.toml` missing | Error: "pixi.toml not found. Was it deleted?" |
| `pixi.lock` missing | Warning: "pixi.lock not found (lock diff unavailable)." |
| Ref not found in registry | Error: "Tag 'v2.0' not found in registry 'ds-team'." |
| Origin digest garbage-collected | Error with suggestion to use `nebi diff --remote` instead |
| Stale index entry (path moved) | Shown as `missing!` in workspace list; `nebi ws prune` to clean |

---

## Implementation Phases

### MVP (Phase 1)

| Feature | Notes |
|---------|-------|
| `nebi status` | Local drift check using .nebi layer digests (offline) |
| `nebi status --remote` | Fetch remote manifest digest, compare |
| `nebi diff` | Fetch original content by digest, show pixi.toml semantic diff |
| `nebi diff --remote`| Compare local vs current tag in registry |
| `nebi diff --json` | JSON output for tooling/CI |
| `nebi push --dry-run` | Preview push using same diff logic |
| `nebi pull --global` | Global pull with duplicate prevention |
| `nebi workspace list --local` | With drift status column |
| `nebi workspace prune` | Clean stale index entries |

### Phase 2

| Feature | Notes |
|---------|-------|
| `nebi diff <ref1> <ref2>` | Compare two registry versions |
| `nebi diff --lock` | Detailed lock file package diff |
| Lock file package-level diff | Parse YAML, compare package sets |
| Manifest caching | `~/.cache/nebi/` with TTL |

### Phase 3 (Future)

| Feature | Notes |
|---------|-------|
| `nebi merge` | Interactive merge of local + remote changes |
| `nebi stash` | Save local changes before pull --force |
| Three-way diff | Show local, remote, and common ancestor |
| `nebi cache prune` | Remove orphaned cached content |
| `nebi workspace sync` | Update all local copies of a workspace |
