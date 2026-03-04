---
sidebar_position: 1
---

# First Steps

This page covers the basics: initializing a workspace, tracking it with Nebi, and using it.

## Create a New Workspace

Start by creating a directory and initializing it:

```bash
mkdir my-project && cd my-project
nebi init
```

If no `pixi.toml` exists, Nebi automatically runs `pixi init` for you:

```
No pixi.toml found; running pixi init...
✔ Created /home/user/my-project/pixi.toml
Workspace 'my-project' initialized (/home/user/my-project)
```

That's it. Nebi reads the workspace name from the `[workspace]` field in your `pixi.toml` and registers it in its local database.

## Track an Existing Pixi Project

Already have a Pixi project? Just run `nebi init` in the directory:

```bash
cd existing-pixi-project
nebi init
```

```
Workspace 'existing-pixi-project' initialized (/home/user/existing-pixi-project)
```

Nebi doesn't modify your `pixi.toml` or `pixi.lock`. It only creates a tracking entry in its own local database (`~/.local/share/nebi/nebi.db` on Linux).

## Use Your Workspace

Once tracked, you can activate the workspace's environment:

```bash
# Open an interactive shell with the workspace's environment
nebi shell my-project

# Run a specific pixi task
nebi run my-project jupyter-lab
```

These commands wrap `pixi shell` and `pixi run` respectively. The key difference is that **you can run them from any directory** — Nebi looks up the workspace path by name and points Pixi to the right `pixi.toml`.

:::note
You still use Pixi directly for adding dependencies, managing environments, and defining tasks. Nebi's `shell` and `run` are convenience wrappers that add name-based workspace lookup.
:::

## What Just Happened?

When you ran `nebi init`, Nebi:

1. Checked for a `pixi.toml` in the current directory (ran `pixi init` if missing)
2. Read the workspace name from `pixi.toml`
3. Stored a tracking entry (name + path) in its local SQLite database

The workspace files (`pixi.toml`, `pixi.lock`, `.pixi/`) are unchanged. Nebi only tracks *where* your workspaces are so it can find them by name later.

## Next Steps

Now that you have a tracked workspace, let's look at how to [manage multiple workspaces](./working-with-workspaces.md).
