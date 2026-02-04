# Nebi CLI Workflows

This guide walks through common nebi CLI workflows with example commands and expected output.

## One-Time Setup: Server Connection

Before syncing workspaces with a team, register and authenticate with your nebi server. You only need to do this once per server.

```bash
# Register a server with a short name
$ nebi server add work https://nebi.company.com
Server "work" added (default)

# List registered servers
$ nebi server list
  NAME   URL                           DEFAULT
  work   https://nebi.company.com      ✓

# Authenticate (opens interactive login prompt)
$ nebi login work
Username: alice
Password: ********
Logged in to "work" as alice
```

The first server you add becomes the default, so you can omit `-s work` on subsequent commands. You can change the default later:

```bash
$ nebi server set-default staging
Default server set to 'staging'
```

---

## Workflow 1: Track a Workspace Locally

Initialize a workspace with nebi. If no `pixi.toml` exists, `nebi init` runs `pixi init` automatically.

```bash
# Create and track a new workspace (runs pixi init if needed)
$ mkdir my-project && cd my-project
$ nebi init
No pixi.toml found; running pixi init...
✔ Created pixi.toml
Workspace "my-project" tracked at /home/alice/my-project

# Add some dependencies
$ pixi add python numpy pandas

# See tracked workspaces
$ nebi workspace list
  NAME          TYPE     PATH
  my-project    local    /home/alice/my-project
```

---

## Workflow 2: Push and Pull Specs

Share workspace specs with your team by pushing to a server, or pull someone else's workspace to reproduce their environment.

### Pushing

```bash
# Push with a version tag
$ nebi push my-project:v1.0
Pushed my-project:v1.0 to server "work"
  pixi.toml  ✓
  pixi.lock  ✓

# Push an updated version after making changes
$ pixi add scipy matplotlib
$ nebi push my-project:v2.0
Pushed my-project:v2.0 to server "work"
```

### Pulling

```bash
# Pull into the current directory
$ nebi pull my-project:v1.0
Pulled my-project:v1.0 → ./
  pixi.toml  ✓
  pixi.lock  ✓

# Pull into a specific directory
$ nebi pull my-project:v1.0 -o ./reproduced-env

# Pull as a global workspace (stored centrally by nebi)
$ nebi pull my-project:v2.0 --global data-science
Pulled my-project:v2.0 → global workspace "data-science"
```

---

## Workflow 3: Browse Server Workspaces

See what workspaces and versions are available on the server.

```bash
# List workspaces on the server
$ nebi workspace list -s work
  NAME           STATUS    OWNER    UPDATED
  my-project     active    alice    2025-06-15 14:22
  ml-pipeline    active    bob      2025-06-10 09:00

# List tags for a specific workspace
$ nebi workspace tags my-project -s work
  TAG     VERSION  CREATED            UPDATED
  v1.0    1        2025-06-01 10:30
  v2.0    2        2025-06-15 14:22
```

---

## Workflow 4: Diff Specs

Compare workspace specs between local directories, server versions, or any combination.

```bash
# Compare two server versions
$ nebi diff my-project:v1.0 my-project:v2.0
--- my-project:v1.0
+++ my-project:v2.0
@@ pixi.toml @@
 [dependencies]
+matplotlib = ">=3.8"
+scipy = ">=1.11"

@@ pixi.lock (changed) @@
  12 packages changed, 12 added
[Use --lock for full lock file details]

# With --lock to see package-level lock diff
$ nebi diff my-project:v1.0 my-project:v2.0 --lock
--- my-project:v1.0
+++ my-project:v2.0
@@ pixi.toml @@
 [dependencies]
+matplotlib = ">=3.8"
+scipy = ">=1.11"

@@ pixi.lock @@
+matplotlib 3.8.5
+scipy 1.11.4
-numpy 1.26.4
+numpy 1.26.5

2 packages added, 1 package updated

# Compare two local directories
$ nebi diff ./project-a ./project-b

# Compare local directory against a server version
$ nebi diff ./my-project my-project:v1.0 -s work

# Compare two global workspaces by name
$ nebi diff data-science ml-pipeline
```

---

## Workflow 5: Global Workspaces, Shell, and Run

Global workspaces live in `~/.local/share/nebi/` and can be activated by name from anywhere.

Both `nebi shell` and `nebi run` auto-initialize untracked pixi directories (equivalent to running `nebi init` first). All arguments after the optional workspace name are passed through directly to `pixi shell` or `pixi run`.

```bash
# Promote the current tracked workspace to a global workspace
$ nebi workspace promote data-science
Promoted /home/alice/my-project → global workspace "data-science"

# List all workspaces (local and global)
$ nebi workspace list
  NAME            TYPE      PATH
  my-project      local     /home/alice/my-project
  data-science    global    /home/alice/.local/share/nebi/data-science

# Open a pixi shell in a global workspace
$ nebi shell data-science

# Open a shell in a specific pixi environment (args pass through to pixi)
$ nebi shell data-science -e jupyter

# Open a shell in the current directory's workspace (auto-initializes if needed)
$ nebi shell

# Run a pixi task in the current directory (auto-initializes if needed)
$ nebi run my-task

# Run a task in a global workspace
$ nebi run data-science my-task

# Run with a specific pixi environment
$ nebi run -e dev my-task

# Run a task in a local directory
$ nebi run ./my-project my-task

# Remove a global workspace
$ nebi workspace remove data-science
Removed workspace "data-science"
```

> **Note**: `--manifest-path` is not supported by `nebi shell` or `nebi run` since nebi manages workspace resolution. Use `pixi shell` or `pixi run` directly if you need `--manifest-path`.

---

## Workflow 6: Origin Tracking and Status

After pushing or pulling, nebi remembers the server, workspace name, and tag as an "origin" (one per server). This enables shorthand commands and sync status checks.

```bash
# Push sets the origin automatically
$ nebi push my-project:v1.0 -s work
Pushed my-project:v1.0 (version 1)

# Check sync status
$ nebi status
Workspace: my-project
Type:      local
Path:      /home/alice/my-project

Origins:
  work → my-project:v1.0 (push, 2025-06-15T14:22:00Z)
    In sync with my-project:v1.0

# Diff against origin (no args needed)
$ nebi diff
--- local
+++ my-project:v1.0
No differences.

# Push a new tag using origin's workspace name
$ nebi push :v2.0
Using workspace "my-project" from origin
Pushing my-project:v2.0...
Pushed my-project:v2.0 (version 2)

# Pull re-fetches from origin (no args needed)
$ nebi pull
Using origin my-project:v2.0 from server "work"
pixi.toml already exists in /home/alice/my-project. Overwrite? [y/N] y
Pulled my-project:v2.0 (version 2) -> /home/alice/my-project

# If someone else force-pushed the same tag, you'll see a warning
$ nebi pull
Using origin my-project:v2.0 from server "work"
Note: my-project:v2.0 has changed on server since last sync
pixi.toml already exists in /home/alice/my-project. Overwrite? [y/N] y
Pulled my-project:v2.0 (version 2) -> /home/alice/my-project
```

---

## Workflow 7: Publish to an OCI Registry

Publish a server-hosted workspace version to an OCI registry for distribution.

```bash
# List available registries on the server
$ nebi registry list -s work
  NAME    URL
  ghcr    ghcr.io

# Publish using the server's default registry
$ nebi publish my-project:v1.0

# Publish with a custom image reference
$ nebi publish my-project:v1.0 myorg/myenv:latest

# Publish to a specific registry
$ nebi publish my-project:v1.0 --registry ghcr myorg/myenv:latest
```

---

## Command Reference

| Group | Command | Description |
|-------|---------|-------------|
| **Workspace** | `nebi init` | Track current directory as a workspace (runs `pixi init` if needed) |
| | `nebi status` | Show workspace sync status |
| | `nebi workspace list [-s server]` | List local, global, or server workspaces |
| | `nebi workspace tags <name> -s server` | List version tags on a server |
| | `nebi workspace promote <name>` | Copy current workspace to a global workspace |
| | `nebi workspace remove <name>` | Remove a workspace from tracking |
| | `nebi shell [name-or-path] [pixi-args...]` | Activate a pixi shell (auto-initializes) |
| | `nebi run [name-or-path] [pixi-args...]` | Run a command or task via pixi (auto-initializes) |
| **Sync** | `nebi push [<name>:]<tag> [-s server]` | Push specs to a server (name from origin if omitted) |
| | `nebi pull [<name>[:<tag>]] [-s server]` | Pull specs from a server (origin if omitted) |
| | `nebi diff [<ref-a>] [ref-b] [-s server]` | Compare workspace specs (origin if omitted) |
| | `nebi publish <name>:<tag> [-s server]` | Publish to an OCI registry |
| **Server** | `nebi server add <name> <url>` | Register a server |
| | `nebi server list` | List registered servers |
| | `nebi server set-default <name>` | Set the default server |
| | `nebi server remove <name>` | Remove a server |
| | `nebi login <server>` | Authenticate with a server |
| | `nebi registry list -s server` | List OCI registries |
| **Admin** | `nebi serve [--port N] [--mode M]` | Run a nebi server instance |
