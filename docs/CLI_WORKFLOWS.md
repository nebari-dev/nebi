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

Start by creating a pixi project, then register it with nebi.

```bash
# Create a new pixi workspace
$ mkdir my-project && cd my-project
$ pixi init
✔ Created pixi.toml

# Add some dependencies
$ pixi add python numpy pandas

# Register with nebi
$ nebi init
Workspace "my-project" tracked at /home/alice/my-project

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

## Workflow 5: Global Workspaces and Shell

Global workspaces live in `~/.local/share/nebi/` and can be activated by name from anywhere.

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

# Open a shell in a specific pixi environment
$ nebi shell data-science -e jupyter

# Open a shell in the current directory's workspace
$ nebi shell

# Remove a global workspace
$ nebi workspace remove data-science
Removed workspace "data-science"
```

---

## Workflow 6: Publish to an OCI Registry

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
| **Workspace** | `nebi init` | Track current directory as a workspace |
| | `nebi workspace list [-s server]` | List local, global, or server workspaces |
| | `nebi workspace tags <name> -s server` | List version tags on a server |
| | `nebi workspace promote <name>` | Copy current workspace to a global workspace |
| | `nebi workspace remove <name>` | Remove a workspace from tracking |
| | `nebi shell [name-or-path] [-e env]` | Activate a pixi shell |
| **Sync** | `nebi push <name>:<tag> [-s server]` | Push specs to a server |
| | `nebi pull <name>[:<tag>] [-s server]` | Pull specs from a server |
| | `nebi diff <ref-a> [ref-b] [-s server]` | Compare workspace specs |
| | `nebi publish <name>:<tag> [-s server]` | Publish to an OCI registry |
| **Server** | `nebi server add <name> <url>` | Register a server |
| | `nebi server list` | List registered servers |
| | `nebi server set-default <name>` | Set the default server |
| | `nebi server remove <name>` | Remove a server |
| | `nebi login <server>` | Authenticate with a server |
| | `nebi registry list -s server` | List OCI registries |
| **Admin** | `nebi serve [--port N] [--mode M]` | Run a nebi server instance |
