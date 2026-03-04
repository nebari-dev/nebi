---
sidebar_position: 4
---

# Compare Environments

`nebi diff` lets you compare `pixi.toml` specs between any combination of local directories, tracked workspaces, and server versions.

## Compare Local Changes Against Last Sync

The most common use case — see what you changed since your last push or pull:

```bash
cd my-project
nebi diff
```

This compares your current local files against the [origin](../concepts/origin-tracking.md) (the version you last pushed or pulled).

## Compare Two Server Versions

See what changed between two tagged versions:

```bash
nebi diff my-project:v1.0 my-project:v2.0
```

## Compare Two Local Directories

Compare two different projects on your machine:

```bash
nebi diff ./project-a ./project-b
```

## Compare Local Against Server

Check how your local version differs from what's on the server:

```bash
nebi diff ./my-project my-project:production
```

## Include Lock File Details

By default, `nebi diff` only shows a summary for lock file changes:

```
@@ pixi.lock (changed) @@
5 packages changed (2 added, 1 removed, 2 updated)
[Use --lock for full lock file details]
```

For the full lock file diff:

```bash
nebi diff --lock
```

## How References Work

Nebi determines the reference type from the format:

| Format | Type |
|--------|------|
| `./path` or `/absolute/path` | Local directory |
| `bare-name` | Tracked workspace name (falls back to server lookup) |
| `name:tag` | Server version at a specific tag |

One argument compares that reference against the current directory. Two arguments compare them against each other. No arguments compares the current directory against its origin.
