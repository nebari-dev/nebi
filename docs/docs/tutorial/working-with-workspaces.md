---
sidebar_position: 2
---

# Working with Workspaces

Once you've initialized a few workspaces, Nebi lets you list, activate, and manage them by name.

## List Your Workspaces

See all workspaces tracked by Nebi:

```bash
nebi workspace list
```

```
NAME             PATH
my-project       /home/user/my-project
ml-pipeline      /home/user/ml-pipeline
data-science     /home/user/data-science
```

If a workspace's directory has been moved or deleted, it shows as `(missing)`. You can clean these up with `nebi workspace prune`.

## Activate by Name

The main convenience of tracking workspaces is activating them from anywhere:

```bash
# From any directory, open a shell in the data-science environment
nebi shell data-science

# Run a task from the ml-pipeline workspace
nebi run ml-pipeline train
```

When you use a workspace name, Nebi passes `--manifest-path` to Pixi pointing at the workspace's `pixi.toml`. Your current working directory stays the same — only the environment changes.

## Activate by Path

You can also reference workspaces by path. Any argument containing `/` is treated as a path:

```bash
# Activate a workspace by relative path
nebi shell ./my-project

# Or absolute path
nebi shell /home/user/data-science
```

When you use a path, Nebi changes to that directory before running Pixi, so your working directory becomes the workspace directory.

## Pass Arguments to Pixi

Anything after the workspace name is forwarded to Pixi:

```bash
# Activate a specific pixi environment
nebi shell data-science -e cuda

# Run a task with extra arguments
nebi run ml-pipeline train -- --epochs 100
```

## Handle Name Conflicts

Multiple workspaces can share the same name (since names come from `pixi.toml`). If there's a conflict, Nebi shows an interactive picker:

```bash
nebi shell data-science
```

```
Multiple workspaces named 'data-science':
  1. /home/user/projects/data-science
  2. /home/user/experiments/data-science
Select workspace [1-2]:
```

In non-interactive contexts (scripts, CI), Nebi errors with the list of paths so you can disambiguate using a path instead.

## Workspace Names Track pixi.toml

If you rename a workspace in `pixi.toml` (by changing the `[workspace] name` field), Nebi automatically detects the change the next time you list or use workspaces:

```
Workspace name updated: "old-name" -> "new-name" (from pixi.toml)
```

## Remove Tracking

To stop tracking a workspace (without deleting any files):

```bash
# Remove the workspace in the current directory
nebi workspace remove .

# Remove by name
nebi workspace remove data-science

# Remove by path
nebi workspace remove /home/user/data-science
```

To clean up all workspaces whose directories no longer exist:

```bash
nebi workspace prune
```

## Next Steps

So far everything has been local to your machine. Next, let's [connect to a server](./sharing-with-a-server.md) to share workspaces with your team.
