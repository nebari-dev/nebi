---
sidebar_position: 2
---

# Start from a Template

You can bootstrap a new project from an existing environment — either from an OCI registry or from a Nebi server.

## From an OCI Registry

If your organization publishes template environments to an OCI registry:

```bash
# Import the template into a new project directory
nebi import ghcr.io/myorg/data-science-template:latest -o ./my-new-project

# Install the packages
cd my-new-project
pixi install
```

You now have a local copy of the template's `pixi.toml` and `pixi.lock`. Edit the `pixi.toml` to customize it for your project — rename the workspace, add or remove dependencies, etc.

## From a Nebi Server

If the template is on your team's Nebi server:

```bash
# Pull the template
nebi pull team-template:stable -o ./my-new-project

# Install the packages
cd my-new-project
pixi install
```

## Customize and Push

After modifying the template for your project, you can push it as a new workspace:

```bash
cd my-new-project

# Edit pixi.toml — change the workspace name, add dependencies
pixi add pandas matplotlib

# Push as a new workspace (the name comes from your pixi.toml)
nebi push my-new-project:v1.0
```

The new workspace is independent of the template — there's no link between them. If the template is updated later, you won't be affected unless you explicitly pull it again.
