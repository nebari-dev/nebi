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

## Publish a Workspace Bundle

`nebi publish --local` pushes the workspace directly to an OCI registry as a
self-contained bundle. The artifact always contains `pixi.toml` and
`pixi.lock`; any additional files in the workspace become opaque asset
layers in the bundle.

```bash
nebi publish --local --registry my-registry --tag v1
```

### Selecting what goes into the bundle

By default the walker includes every regular file in the workspace
directory, with these rules applied in order:

1. `.git/` and `.pixi/` are always dropped.
2. If `[tool.nebi.bundle].include` is set in `pixi.toml`, only files
   matching those globs are candidates.
3. Files matched by `.gitignore` are dropped.
4. Files matched by `[tool.nebi.bundle].exclude` are dropped.
5. `pixi.toml` and `pixi.lock` are force-included even if earlier rules
   would have dropped them.
6. Symlinks, devices, and FIFOs are skipped silently.

Configure the filters from `pixi.toml`:

```toml
[tool.nebi.bundle]
include = ["src/**", "assets/**", "README.md"]
exclude = ["*.log", "secrets/**", "notes.md"]
```

`include` and `exclude` work together: `include` whitelists candidates,
and `exclude` further narrows them down. When only `exclude` is set,
everything is a candidate minus the excluded paths.

### Parallelism

`--concurrency N` bounds parallel blob pushes. Default is 8. Raise it for
high-latency registries; lower it to be gentle on constrained networks.

## Import from an OCI Registry

Pull a workspace bundle from an OCI registry. The core files (`pixi.toml`,
`pixi.lock`) are always restored; any asset layers in the bundle are
extracted to the output directory at their original relative paths.

```bash
nebi import quay.io/nebari/data-science:v1.0 -o ./my-project
```

```bash title="Output"
Tracking workspace 'data-science' at /home/user/my-project
Imported quay.io/nebari/data-science:v1.0 -> /home/user/my-project (3 asset file(s))
```

Use `--concurrency N` to tune parallel blob fetches (default 8).

When a bundle contains asset layers, the output directory must not
already exist or must be empty — imports do not overwrite existing
files. Legacy two-layer artifacts (no assets) keep the `--force` escape
hatch.

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

:::note
This only removes the local tracking entry. Your project files are untouched. To delete a workspace from the server, use `--remote`:
```bash
nebi workspace remove my-workspace --remote
```
:::

To clean up all workspaces whose directories no longer exist:

```bash
nebi workspace prune
```
