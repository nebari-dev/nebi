---
sidebar_position: 5
---

# Status and Diffing

Nebi tracks what you've pushed and pulled so it can tell you when things have changed. This page covers `nebi status` and `nebi diff`.

## Check Status

After you've pushed or pulled a workspace, Nebi stores an **origin** — a record of what was synced and when. The `status` command uses this to show you what's changed:

```bash
cd my-project
nebi status
```

```
Workspace: my-project
Path:      /home/user/my-project
Server:    https://nebi.company.com

pixi.toml modified locally

Origin:
  my-project:v1.0 (push)
  In sync with my-project:v1.0
```

This tells you:

- Your local `pixi.toml` has changed since your last push
- The server version hasn't changed (no one else has pushed)

If you're not connected to a server, status only shows local modifications.

### Possible Server States

| Status | Meaning |
|--------|---------|
| In sync with workspace:tag | Server version matches what you last synced |
| workspace:tag has changed on server | Someone else pushed new content |
| Workspace not found on server | Workspace was deleted from server |
| Tag not found on server | The specific tag was removed |
| Server not reachable | Network issue |
| Not logged in | No server credentials configured |

## Compare Environments

The `diff` command lets you compare `pixi.toml` specs between any two references — local directories, tracked workspace names, or server versions.

### Compare Against Your Last Sync

With no arguments, `diff` compares your current local files against the last version you pushed or pulled:

```bash
cd my-project
nebi diff
```

This is the most common usage — "what did I change since my last sync?"

### Compare Two Server Versions

```bash
nebi diff my-project:v1.0 my-project:v2.0
```

### Compare Two Local Directories

```bash
nebi diff ./project-a ./project-b
```

### Compare Local Against Server

```bash
nebi diff ./my-project my-project:production
```

### Include Lock File Details

By default, diff shows a summary for lock file changes:

```
@@ pixi.lock (changed) @@
5 packages changed (2 added, 1 removed, 2 updated)
[Use --lock for full lock file details]
```

Add `--lock` for the full lock file diff:

```bash
nebi diff --lock
```

### How References Are Resolved

| Format | Resolved as |
|--------|-------------|
| `./path` or `/absolute/path` | Local directory |
| `bare-name` | Tracked workspace name (falls back to server) |
| `name:tag` | Server workspace at specific tag |

## Next Steps

You've now covered the core local and server workflows. Next, learn how to [publish environments to OCI registries](./publishing-to-oci.md) for broader distribution.
