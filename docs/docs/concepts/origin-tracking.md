---
sidebar_position: 4
---

# Origin Tracking

When you push or pull a workspace, Nebi records an **origin** — metadata about what was synced, when, and in which direction. Origins enable several convenience features and are key to how `nebi status` and `nebi diff` work.

## What an Origin Stores

After a push or pull, Nebi saves:

| Field | Example | Purpose |
|-------|---------|---------|
| Origin name | `my-project` | The server workspace name |
| Origin tag | `v1.0` | The tag that was pushed or pulled |
| Origin action | `push` or `pull` | Which direction the sync went |
| TOML hash | `sha256:...` | Hash of `pixi.toml` at sync time |
| Lock hash | `sha256:...` | Hash of `pixi.lock` at sync time |

This is similar to how Git tracks which remote branch a local branch is associated with.

## What Origins Enable

### Argument-Free Commands

Once an origin is set, you can omit the workspace name in push, pull, and diff:

```bash
# First push sets the origin
nebi push my-project:v1.0

# Subsequent pushes reuse it
nebi push          # pushes to my-project
nebi push :v1.1    # pushes to my-project with a new tag

# Same for pull
nebi pull my-project:v1.0
nebi pull          # re-pulls my-project:v1.0
```

### Local Modification Detection

`nebi status` compares the current `pixi.toml` and `pixi.lock` against the hashes stored in the origin. If the files have changed since the last sync, status reports it:

```
pixi.toml modified locally
```

The comparison is **semantic** for TOML files — whitespace and formatting changes are ignored. Only meaningful content changes are reported.

### Server Divergence Detection

When connected to a server, `nebi status` also checks whether the server version has changed since your last sync. This tells you if a teammate has pushed new content:

```
my-project:v1.0 has changed on server since last sync
```

### Baseline for Diffs

`nebi diff` with no arguments compares your current local files against the origin version. This answers "what did I change since my last sync?"

## When Origins Are Set

Origins are set automatically by:

- **`nebi push`** — records the workspace name, tag, and file hashes
- **`nebi pull`** — records the workspace name, tag, and file hashes from the server
- **`nebi import`** — does not set a server origin (since imports come from OCI registries, not a Nebi server)

## Viewing Origin Information

```bash
nebi status
```

The "Origin" section at the bottom shows the current origin:

```
Origin:
  my-project:v1.0 (push)
  In sync with my-project:v1.0
```

If no origin is set (workspace was only initialized locally), the origin section is omitted.
