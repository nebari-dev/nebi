---
sidebar_position: 3
---

# CLI Workflows

This guide walks through common Nebi CLI workflows with example commands and expected output.

## One-Time Setup: Server Connection

Before syncing workspaces with a team, register and authenticate with your Nebi server. You only need to do this once per server.

```bash
# Authenticate with a server
$ nebi login https://nebi.company.com
Username: alice
Password: ********
Logged in to "https://nebi.company.com" as alice
```

---

## Workflow 1: Track a Workspace Locally

Initialize a workspace with Nebi. If no `pixi.toml` exists, `nebi init` runs `pixi init` automatically.

```bash
# Create and track a new workspace (runs pixi init if needed)
$ mkdir my-project && cd my-project
$ nebi init
No pixi.toml found; running pixi init...
Created pixi.toml
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
Pushed my-project:v1.0 to server
  pixi.toml  OK
  pixi.lock  OK

# Push an updated version after making changes
$ pixi add scipy matplotlib
$ nebi push my-project:v2.0
Pushed my-project:v2.0 to server
```

### Pulling

```bash
# Pull into the current directory
$ nebi pull my-project:v1.0
Pulled my-project:v1.0

# Pull into a specific directory
$ nebi pull my-project:v1.0 -o ./reproduced-env
```

---

## Workflow 3: Diff Specs

Compare workspace specs between local directories, server versions, or any combination.

```bash
# Compare two server versions
$ nebi diff my-project:v1.0 my-project:v2.0

# Compare two local directories
$ nebi diff ./project-a ./project-b

# Compare local directory against a server version
$ nebi diff ./my-project my-project:v1.0
```

---

## Workflow 4: Global Workspaces, Shell, and Run

Global workspaces live in `~/.local/share/nebi/` and can be activated by name from anywhere.

```bash
# Promote the current tracked workspace to a global workspace
$ nebi workspace promote data-science

# Open a pixi shell in a global workspace
$ nebi shell data-science

# Run a pixi task in the current directory
$ nebi run my-task

# Run a task in a global workspace
$ nebi run data-science my-task
```

---

## Workflow 5: Publish to an OCI Registry

Publish a server-hosted workspace version to an OCI registry for distribution.

```bash
# List available registries on the server
$ nebi registry list
  NAME    URL
  ghcr    ghcr.io

# Publish using the server's default registry
$ nebi publish my-project:v1.0

# Publish with a custom image reference
$ nebi publish my-project:v1.0 myorg/myenv:latest
```
