---
sidebar_position: 3
---

# Share with a Teammate

To share your environment with a teammate, push it to a Nebi server. They can then pull it to their machine.

## Push Your Environment

```bash
cd my-project

# Log in (one-time)
nebi login https://nebi.company.com

# Make sure your workspace is tracked
nebi init

# Push to the server with a tag
nebi push my-project:v1.0
```

This uploads your `pixi.toml` and `pixi.lock` to the server. If the workspace doesn't exist on the server yet, Nebi creates it automatically.

## Tell Your Teammate

Your teammate needs:

1. The server URL: `https://nebi.company.com`
2. The workspace name and tag: `my-project:v1.0`

They run:

```bash
nebi login https://nebi.company.com
nebi pull my-project:v1.0 -o ./my-project
cd my-project
pixi install
```

## Update the Shared Environment

When you make changes and want to share them:

```bash
# Make changes with pixi
pixi add scikit-learn

# Push the update
nebi push my-project:v1.1
```

Your teammate can check for updates with `nebi status` and pull the new version:

```bash
nebi pull my-project:v1.1
pixi install
```

## Use Tags for Stability

If you want teammates to always get a stable version while you iterate, use separate tags:

```bash
# Push work-in-progress
nebi push my-project:dev

# When you're happy with it, push with the stable tag
nebi push my-project:stable
```

Tell your team to pull `:stable`. When you push a new version with the same tag, it updates the pointer.
