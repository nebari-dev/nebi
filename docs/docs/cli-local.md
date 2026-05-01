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

`nebi publish` packages your workspace and pushes it to an OCI registry.
Every bundle includes `pixi.toml` and `pixi.lock`, plus any other
workspace files (READMEs, source code, data) as additional layers.

```bash
nebi publish --registry my-registry --tag v1
```

### Selecting what goes into the bundle

By default, the bundle includes everything in your workspace except
`.git/` and `.pixi/`. `pixi.toml` and `pixi.lock` are always included
no matter what.

To customize what gets bundled, add a `[tool.nebi.bundle]` table to
`pixi.toml`:

```toml
[tool.nebi.bundle]
include = ["src/**", "assets/**", "README.md"]
exclude = ["*.log", "secrets/**", "notes.md"]
```

- **`include`**: turn the default into a strict allowlist. Only files
  matching these patterns are kept.
- **`exclude`**: drop additional files (for example, `nebi.db*`) from
  what's been kept so far.

`.gitignore` rules also apply: files git ignores are kept out of
bundles. Symlinks, device files, and named pipes are skipped silently.

### Parallelism

`--concurrency N` sets how many files upload or download at the same
time. Default is 8. Raise it (e.g., 16 or 32) when the registry is slow
to respond. Lower it (e.g., 2 or 4) on slow or rate-limited networks.

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

Use `--concurrency N` to set how many files download at the same time (default 8).

:::note Imports do not overwrite existing files
If the bundle includes asset files, `nebi import` refuses to write
into a non-empty output directory. Use `-o ./some-new-dir` to land
it in a fresh folder.

Older bundles that contain only `pixi.toml` and `pixi.lock` still
accept `--force` to overwrite.
:::

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
