---
sidebar_position: 1
---

# Workspaces

A **workspace** in Nebi is a tracked Pixi project — a directory containing a `pixi.toml` (and usually a `pixi.lock`).

## Workspaces in Pixi vs Nebi

In Pixi, a workspace is a project directory defined by a `pixi.toml` file. It specifies dependencies, environments, and tasks. Pixi handles creating the environment and installing packages.

Nebi uses the exact same concept. A Nebi workspace *is* a Pixi workspace — Nebi just adds tracking, naming, and syncing on top. When you run `nebi init`, you're telling Nebi "remember this Pixi project so I can reference it by name and sync it later."

Nebi does not introduce a new project format. Your `pixi.toml` and `pixi.lock` are standard Pixi files.

## Workspace Names

Every workspace has a name, which comes from the `[workspace]` field in `pixi.toml`:

```toml
[workspace]
name = "data-science"
channels = ["conda-forge"]
platforms = ["linux-64"]
```

Nebi reads this name and uses it for tracking and lookup. If you change the name in `pixi.toml`, Nebi detects the change automatically the next time you list or use workspaces.

## Local Tracking

When you run `nebi init`, Nebi stores a tracking entry in a local SQLite database (`~/.local/share/nebi/nebi.db` on Linux). This entry maps the workspace name to its filesystem path.

The tracking entry is lightweight — it contains:

- **Name** (from `pixi.toml`)
- **Path** (absolute directory path)
- **Origin metadata** (set after push/pull — see [Origin Tracking](./origin-tracking.md))

Nebi does not modify your project files. Removing tracking (`nebi workspace remove`) doesn't delete any files.

## Name-Based Lookup

Once tracked, you can reference workspaces by name from any directory:

```bash
nebi shell data-science    # from anywhere
nebi run data-science test # from anywhere
```

Nebi resolves the name to a path, then either:

- Sets `--manifest-path` to point Pixi at the right `pixi.toml` (for name-based lookup)
- Changes to the directory (for path-based lookup)

## Multiple Workspaces with the Same Name

Since names come from `pixi.toml`, it's possible to have multiple tracked workspaces with the same name (for example, two different projects that happen to both be called "analysis"). When this happens:

- **Interactive terminal:** Nebi shows a numbered picker to let you choose
- **Non-interactive (scripts, CI):** Nebi prints all matching paths and errors, asking you to use a path instead

To avoid ambiguity, use paths or rename one of the workspaces in its `pixi.toml`.

## Server Workspaces

When you push a workspace to a Nebi server, it creates a server-side workspace entry. Server workspace names are unique (unlike local names, which can have duplicates). The server stores versioned copies of `pixi.toml` and `pixi.lock` — see [Versions and Tags](./versions-and-tags.md) for details.
