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

**Push** uploads your local `pixi.toml` and `pixi.lock` to the Nebi server. This is how you share workspace specs with your team, or stage them for publishing to an OCI registry.

Every push automatically creates a **content-addressed tag** (`sha-<hash>`) and updates a **`latest`** tag. If you specify a tag, it's added alongside these auto-tags. If the content hasn't changed since the last push, the version is **deduplicated** (no new version created, tags are updated).

### Pushing

```bash
# Push (auto-tags with content hash + latest)
$ nebi push my-project
Pushed my-project (version 1, tags: sha-a1b2c3d4e5f6, latest)

# Push with an explicit user tag
$ nebi push my-project:v1.0
Pushed my-project (version 1, tags: sha-a1b2c3d4e5f6, latest, v1.0)

# Push again without changes (deduplicated)
$ nebi push my-project
Content unchanged — my-project (version 1, tags: sha-a1b2c3d4e5f6, latest)

# Push after making changes
$ pixi add scipy matplotlib
$ nebi push my-project:v2.0
Pushed my-project (version 2, tags: sha-f8426b81dfed, latest, v2.0)
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

**Publish** takes the workspace files already on the Nebi server and pushes them to an external OCI registry (e.g., Quay.io, GHCR) for distribution. You must **push** before you **publish** — publish reads from the server, not your local files.

By default, the content hash tag is used as the primary OCI tag, and a `latest` tag is always created. All workspace tags (user tags, content hash, latest) are propagated to the OCI registry.

```bash
# Typical workflow: push local changes, then publish to OCI
$ nebi push my-project
Pushed my-project (version 2, tags: sha-f8426b81dfed, latest)
$ nebi publish my-project
Published my-project-8b3fd00c:sha-f8426b81dfed

# List available registries on the server
$ nebi registry list
  NAME    URL
  ghcr    ghcr.io

# Publish with a custom OCI tag
$ nebi publish my-project --tag v1.0.0

# Publish to a specific registry and repository
$ nebi publish my-project --registry ghcr --repo myorg/myenv
```

---

## Workflow 6: Import from an OCI Registry

Import a workspace into nebi from a public OCI registry.

```bash
# Import a workspace into the current directory
$ nebi import quay.io/nebari/my-env:v1
Imported quay.io/nebari/my-env:v1 -> /home/alice/my-project

# Import into a specific directory
$ nebi import ghcr.io/myorg/data-science:latest -o ./my-project

# Import as a global workspace
$ nebi import quay.io/nebari/my-env:v1 --global data-science
Imported quay.io/nebari/my-env:v1 -> global workspace "data-science" (/home/alice/.local/share/nebi/data-science)

# Overwrite existing files without prompting
$ nebi import quay.io/nebari/my-env:v1 --force
```
