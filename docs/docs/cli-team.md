# Team CLI Workflows

This page covers working with a Nebi server to share workspaces with your team.

## Prerequisite: Nebi server

If you don't have a server for your team or organization yet, see [Server Setup](./server-setup.md).

## Connect to a Server

Before syncing workspaces, connect and authenticate with your Nebi server. You only need to do this once per server.

```bash
$ nebi login https://nebi.company.com
Username: alice
Password: ********
Logged in to "https://nebi.company.com" as alice
```

## Server Push and Pull

**Push** uploads your local `pixi.toml` and `pixi.lock` to the Nebi server. This is how you share workspace specs with your team, or stage them for publishing to an OCI registry.

Every push automatically creates a **content-addressed tag** (`sha-<hash>`) and updates a **`latest`** tag. If you specify a tag, it's added alongside these auto-tags. If the content hasn't changed since the last push, the version is **deduplicated** (no new version created, tags are updated).

### Push

`push` sends your local `pixi.toml` and `pixi.lock` to the server:

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

# After the first push, you can omit the workspace name
$ nebi push :dev
```

### Pull

`pull` downloads `pixi.toml` and `pixi.lock` from the server into a local directory:

```bash
# Pull into the current directory
$ nebi pull my-project:v1.0
Pulled my-project:v1.0

# Pull into a specific directory
$ nebi pull my-project:v1.0 -o ./reproduced-env
```

After pulling, the workspace is automatically tracked by Nebi. Future pulls can omit the workspace name:

```bash
# Re-pull the same workspace with just:
$ nebi pull
```

## Browse Remote Workspaces

```bash
$ nebi workspace list --remote
NAME             STATUS  OWNER  UPDATED
my-data-project  ready   alice  2024-01-15 14:22
ml-pipeline      ready   alice  2024-01-14 10:30
shared-env       ready   bob    2024-01-13 09:15
```

View available tags for a workspace:

```bash
$ nebi workspace tags my-data-project
TAG               VERSION  CREATED           UPDATED
prod              2        2024-01-15 14:22
latest            2        2024-01-15 10:30  2024-01-15 14:22
dev               1        2024-01-15 10:30
sha-b2c3d4e5f6a7  2        2024-01-15 14:22
sha-a1b2c3d4e5f6  1        2024-01-15 10:30
```

## Diff and Status

### Check for Changes

See if your local workspace has diverged from the server:

```bash
$ nebi status
Workspace: my-data-project
Path:      /home/user/my-data-project
Server:    https://nebi.company.com

pixi.toml modified locally

Origin:
  my-data-project:prod (push)
```

### Compare Changes

```bash
# Compare local workspace against its server origin
$ nebi diff

# Compare two server versions
$ nebi diff my-project:v1.0 my-project:v2.0

# Compare two local directories
$ nebi diff ./project-a ./project-b

# Compare a local directory against a server version
$ nebi diff ./my-project my-project:v1.0

# Include lock file changes
$ nebi diff --lock
```

## Publish to an OCI Registry

**Publish** takes the workspace files already on the Nebi server and pushes them to an external OCI registry (e.g., Quay.io, GHCR) for distribution. You must **push** before you **publish** — publish reads from the server, not your local files.

By default, the content hash tag is used as the primary OCI tag, and a `latest` tag is always created. All workspace tags are propagated to the OCI registry.

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
