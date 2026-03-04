---
sidebar_position: 6
---

# Publishing to OCI

Publishing takes workspace specs from your Nebi server and pushes them to an OCI registry (like GitHub Container Registry or Quay.io) for broad distribution. Published environments can be imported by anyone, even without access to your server.

## The Publish Workflow

Publishing is a two-step process:

1. **Push** your workspace to the Nebi server (stores the specs)
2. **Publish** from the server to an OCI registry (distributes the specs)

```bash
# Step 1: Push to server
nebi push my-project
# Pushed my-project (version 2, tags: sha-f8426b81dfed, latest)

# Step 2: Publish to OCI registry
nebi publish my-project
# Published my-project-8b3fd00c:sha-f8426b81dfed
```

:::note
`nebi publish` reads from the server, not your local files. Always push first so the server has the latest version.
:::

## Configure Registries

Your Nebi server administrator configures which OCI registries are available. List them with:

```bash
nebi registry list
```

```
NAME    URL
ghcr    ghcr.io
```

If you're an admin, you can add registries:

```bash
nebi registry add --name ghcr --url ghcr.io --namespace myorg --username myuser --password-stdin
```

## Publish with Custom Tags

```bash
# Publish with a specific OCI tag
nebi publish my-project --tag v1.0.0

# Publish to a specific registry and repository
nebi publish my-project --registry ghcr --repo myorg/data-science-env
```

## Import Published Environments

Anyone can import from a public OCI registry — no Nebi server needed:

```bash
nebi import ghcr.io/myorg/data-science-env:v1.0.0 -o ./my-env
```

```
Tracking workspace 'data-science-env' at /home/user/my-env
Imported ghcr.io/myorg/data-science-env:v1.0.0 -> /home/user/my-env
```

The imported workspace is automatically tracked by Nebi. Run `pixi install` in the directory to install the packages.

## What Gets Published

Nebi publishes `pixi.toml` and `pixi.lock` as an OCI artifact with custom media types:

- `application/vnd.pixi.toml.v1+toml` for the manifest
- `application/vnd.pixi.lock.v1+yaml` for the lock file

This means any OCI-compliant registry can host Nebi environments, and any tool that understands these media types can consume them.

## Next Steps

Finally, let's bring it all together and look at [team workflows](./working-as-a-team.md).
