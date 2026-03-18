# Local CLI Workflows

Nebi manages Pixi workspace specs locally, and syncs them to remote servers. This guide covers local workflows.

> **Note:** Nebi currently only supports `pixi.toml` manifests. Pixi projects using `pyproject.toml` (with `[tool.pixi.*]` tables) are not yet supported.

## Track a New Workspace

Create a new Pixi workspace and start tracking it with Nebi:

```bash
mkdir my-data-project && cd my-data-project
nebi init
```

If no `pixi.toml` exists, Nebi automatically runs `pixi init` for you.

The workspace name comes from the `[workspace] name` field in `pixi.toml`:

```bash title="Output"
No pixi.toml found; running pixi init...
✔ Created /home/user/my-data-project/pixi.toml
Workspace 'my-data-project' initialized (/home/user/my-data-project)
```

## Track an Existing Pixi Workspace

Already have a Pixi project? Just run `nebi init` in the directory:

```bash
cd existing-pixi-project
nebi init
```

```bash title="Output"
Workspace 'existing-pixi-project' initialized (/home/user/existing-pixi-project)
```

:::tip
If you rename a workspace in `pixi.toml` (by changing the `[workspace] name` field), Nebi automatically detects the change the next time you list or use workspaces.
:::

## List Your Workspaces

See all workspaces tracked by Nebi:

```bash
nebi workspace list
```

```bash title="Output"
NAME             PATH
my-data-project  /home/user/my-data-project
ml-pipeline      /home/user/ml-pipeline
data-science     /home/user/data-science
```

## Use (activate) Workspaces

### Activate by Name

Tracked workspaces can be activated from any directory by name or by path

```bash
# Activate a Pixi shell with the workspace's name
nebi shell data-science

# Run a (Pixi) task from a workspace (stays in current directory)
nebi run data-science jupyter-lab
```

If multiple workspaces share the same name, an interactive picker is shown.

### Activate by Path

```bash
# Activate a workspace by relative path
nebi shell ./my-project

# Or, by absolute path
nebi shell /home/user/data-science
```

### Pass Arguments to Pixi

Anything after the workspace name is forwarded to Pixi:

```bash
# Activate a specific pixi environment
nebi shell data-science -e cuda

# Run a task with extra arguments
nebi run ml-pipeline train -- --epochs 100
```

## Import from an OCI Registry

Pull a workspace from a public OCI registry (no server needed):

```bash
nebi import quay.io/nebari/data-science:v1.0 -o ./my-project
```

```bash title="Output"
Tracking workspace 'data-science' at /home/user/my-project
Imported quay.io/nebari/data-science:v1.0 -> /home/user/my-project
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

## Quick Reference

| Task | Command |
|------|---------|
| Track a workspace | `nebi init` |
| List local workspaces | `nebi workspace list` |
| Activate a shell | `nebi shell <name>` |
| Run a task | `nebi run <name> <task>` |
| Import from OCI | `nebi import quay.io/org/env:tag` |
| Connect to a server | `nebi login <server-url>` |
| Push to server | `nebi push myworkspace:prod` |
| Pull from server | `nebi pull myworkspace:prod` |
| List remote workspaces | `nebi workspace list --remote` |
| Check status | `nebi status` |
| Compare changes | `nebi diff` |
| Publish to OCI | `nebi publish myworkspace --tag v1.0` |
