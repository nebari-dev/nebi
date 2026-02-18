# CLI Quickstart

Nebi manages Pixi workspace specs locally and syncs them to remote servers. This guide covers the most common workflows.

> **Note:** Nebi currently only supports `pixi.toml` manifests. Pixi projects using `pyproject.toml` (with `[tool.pixi.*]` tables) are not yet supported.

## Local Use

Nebi can be used entirely locally to track workspaces across your machine and publish to/import from OCI registries - no server required.

### Track a New Workspace

Create a new Pixi workspace and start tracking it with Nebi:

```bash
mkdir my-data-project && cd my-data-project
nebi init
```

If no `pixi.toml` exists, Nebi automatically runs `pixi init` for you:

```
No pixi.toml found; running pixi init...
✔ Created /home/user/my-data-project/pixi.toml
Workspace 'my-data-project' initialized (/home/user/my-data-project)
```

### Track an Existing Pixi Workspace

Already have a Pixi project? Just run `nebi init` in the directory:

```bash
cd existing-pixi-project
nebi init
```

```
Workspace 'existing-pixi-project' initialized (/home/user/existing-pixi-project)
```

### Check Workspace Status

See the current state of your workspace:

```bash
nebi status
```

```
Workspace: my-data-project
Type:      local
Path:      /home/user/my-data-project

No origin. Push or pull to set an origin.
```

### List Your Workspaces

See all workspaces tracked by Nebi:

```bash
nebi workspace list
```

```
NAME             TYPE    PATH
my-data-project  local   /home/user/my-data-project
ml-pipeline      local   /home/user/ml-pipeline
data-science     global  /home/user/.local/share/nebi/workspaces/data-science
```

### Import from an OCI Registry

Pull a workspace from a public OCI registry (no server needed):

```bash
nebi import quay.io/nebari/data-science:v1.0 -o ./my-project
```

```
Tracking workspace 'my-project' at /home/user/my-project
Imported quay.io/nebari/data-science:v1.0 -> /home/user/my-project
```

Or save it as a global workspace you can use from anywhere:

```bash
nebi import quay.io/nebari/data-science:v1.0 --global data-science
```

### Use Global Workspaces

Global workspaces are stored centrally and can be activated from any directory:

```bash
# Activate a shell with the global workspace's environment
nebi shell data-science

# Run a task from a global workspace (stays in current directory)
nebi run data-science jupyter-lab
```

---

## Use with a Nebi Server

Connect to a Nebi server to share workspaces with your team, version them, and publish to OCI registries.

### Connect to a Server

```bash
nebi login https://nebi.company.com
```

```
Username: alice
Password:
Logged in to https://nebi.company.com
```

### Push a Workspace

Push your local workspace to the server:

```bash
cd my-data-project
nebi push my-data-project
```

```
Creating workspace "my-data-project"...
Created workspace "my-data-project"
Pushing my-data-project...
Pushed my-data-project (version 1, tags: sha-a1b2c3d4e5f6, latest)
```

Add a tag (tags are movable labels that can be any string):

```bash
nebi push my-data-project:prod
```

```
Pushing my-data-project:prod...
Pushed my-data-project (version 2, tags: sha-b2c3d4e5f6a7, latest, prod)
```

After the first push, you can omit the workspace name:

```bash
# Make some changes to pixi.toml...
nebi push :dev
```

### Browse Remote Workspaces

List workspaces on the server:

```bash
nebi workspace list --remote
```

```
NAME             STATUS  OWNER  UPDATED
my-data-project  ready   alice  2024-01-15 14:22
ml-pipeline      ready   alice  2024-01-14 10:30
shared-env       ready   bob    2024-01-13 09:15
```

View available tags for a workspace:

```bash
nebi workspace tags my-data-project
```

```
TAG               VERSION  CREATED           UPDATED
prod              2        2024-01-15 14:22
latest            2        2024-01-15 10:30  2024-01-15 14:22
dev               1        2024-01-15 10:30
sha-b2c3d4e5f6a7  2        2024-01-15 14:22
sha-a1b2c3d4e5f6  1        2024-01-15 10:30
```

### Pull a Workspace

Pull a workspace from the server:

```bash
nebi pull my-data-project:prod -o ./local-copy
```

```
Tracking workspace 'local-copy' at /home/user/local-copy
Pulled my-data-project:prod (version 2) -> /home/user/local-copy
```

After pulling, you can re-pull with just:

```bash
nebi pull
```

### Check for Changes

See if your local workspace has diverged from the server:

```bash
nebi status
```

```
Workspace: my-data-project
Type:      local
Path:      /home/user/my-data-project
Server:    https://nebi.company.com

pixi.toml modified locally

Origin:
  my-data-project:prod (push)
```

### Compare Changes

See exactly what changed:

```bash
nebi diff
```

```
--- my-data-project:prod
+++ local
@@ pixi.toml @@
 [dependencies]
+numpy = ">=1.26"
+pandas = ">=2.0"
```

Compare two server versions:

```bash
nebi diff my-data-project:prod my-data-project:dev
```

Include lock file changes:

```bash
nebi diff --lock
```

### Publish to an OCI Registry

Publish a workspace from the server to an OCI registry:

```bash
nebi publish my-data-project --tag v1.0.0
```

```
Publishing my-data-project to quay.io/myorg/my-data-project:v1.0.0...
Published my-data-project (digest: sha256:abc123...)
```

---

## Quick Reference

| Task | Command |
|------|---------|
| Track a workspace | `nebi init` |
| Check status | `nebi status` |
| List local workspaces | `nebi workspace list` |
| List remote workspaces | `nebi workspace list --remote` |
| Push to server | `nebi push myworkspace:prod` |
| Pull from server | `nebi pull myworkspace:prod` |
| Compare changes | `nebi diff` |
| Import from OCI | `nebi import quay.io/org/env:tag` |
| Publish to OCI | `nebi publish myworkspace --tag v1.0` |

For detailed help on any command:

```bash
nebi [command] --help
```