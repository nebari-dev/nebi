---
sidebar_position: 3
---

# Sharing with a Server

A Nebi server lets you share workspace specs with your team. This page covers connecting, pushing, and pulling.

## Connect to a Server

Authenticate with your team's Nebi server:

```bash
nebi login https://nebi.company.com
```

By default, this opens your browser for authentication. You can also use username/password:

```bash
nebi login https://nebi.company.com --username alice
```

Nebi prompts for your password interactively. For scripts and CI, you can pass a token directly:

```bash
nebi login https://nebi.company.com --token <api-token>
```

You only need to log in once — credentials are stored locally.

## Push Your Workspace

Push sends your local `pixi.toml` and `pixi.lock` to the server:

```bash
cd my-project
nebi push my-project
```

```
Pushed my-project (version 1, tags: sha-a1b2c3d4e5f6, latest)
```

This creates the workspace on the server if it doesn't already exist. The server automatically runs `pixi install` to validate the spec.

Every push gets two automatic tags:
- **`sha-<hash>`** — a content-addressed tag based on the file contents
- **`latest`** — always points to the most recent push

You can also add your own tag:

```bash
nebi push my-project:v1.0
```

```
Pushed my-project (version 1, tags: sha-a1b2c3d4e5f6, latest, v1.0)
```

After the first push, you can omit the workspace name since Nebi remembers the origin:

```bash
# Push with a new tag (uses the same workspace)
nebi push :staging
```

## Pull a Workspace

Pull downloads `pixi.toml` and `pixi.lock` from the server into a local directory:

```bash
# Pull into the current directory
nebi pull my-project:v1.0

# Pull into a specific directory
nebi pull my-project:v1.0 -o ./reproduced-env

# Omit the tag to get the latest version
nebi pull my-project
```

After pulling, the workspace is automatically tracked by Nebi. Future pulls can omit the workspace name:

```bash
# Re-pull the same workspace (uses stored origin)
nebi pull
```

If `pixi.toml` already exists in the target directory, Nebi asks for confirmation before overwriting (use `--force` to skip the prompt).

## Browse Remote Workspaces

See what's available on the server:

```bash
nebi workspace list --remote
```

```
NAME             STATUS  OWNER  UPDATED
my-project       ready   alice  2024-01-15 14:22
shared-env       ready   bob    2024-01-13 09:15
```

View the tags for a specific workspace:

```bash
nebi workspace tags my-project
```

```
TAG               VERSION  CREATED           UPDATED
v1.0              1        2024-01-15 10:30
latest            2        2024-01-15 10:30  2024-01-15 14:22
sha-b2c3d4e5f6a7  2        2024-01-15 14:22
sha-a1b2c3d4e5f6  1        2024-01-15 10:30
```

## What Gets Synced

Nebi only syncs **two files**: `pixi.toml` and `pixi.lock`. It does not sync:

- Installed packages (the `.pixi/` directory)
- Source code or data files
- Any other project files

After pulling, you'll need to run `pixi install` to actually install the packages locally. The lock file ensures you get the exact same versions.

## Next Steps

Now that you can push and pull, let's learn about how [versions and tags](./versions-and-tags.md) work.
