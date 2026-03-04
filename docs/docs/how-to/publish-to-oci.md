---
sidebar_position: 6
---

# Publish to an OCI Registry

Publishing distributes your environment specs through an OCI container registry so anyone can import them — even without access to your Nebi server.

## Prerequisites

- A workspace pushed to a Nebi server (`nebi push`)
- An OCI registry configured on the server (ask your admin, or see below)

## Publish a Workspace

```bash
# Push your latest changes to the server first
nebi push my-project

# Publish to the default registry
nebi publish my-project
```

Nebi automatically assigns an OCI tag based on the content hash. You can also specify a custom tag:

```bash
nebi publish my-project --tag v1.0.0
```

## Specify a Registry and Repository

```bash
# List available registries
nebi registry list

# Publish to a specific registry and repo
nebi publish my-project --registry ghcr --repo myorg/data-science-env
```

## Add an OCI Registry (Admin)

If you manage the Nebi server, you can add registries:

```bash
nebi registry add \
  --name ghcr \
  --url ghcr.io \
  --namespace myorg \
  --username myuser \
  --password-stdin <<< "$GHCR_TOKEN"
```

Remove a registry:

```bash
nebi registry remove ghcr
```

## Import a Published Environment

Anyone can import from a public OCI registry:

```bash
nebi import ghcr.io/myorg/data-science-env:v1.0.0 -o ./my-env
cd my-env
pixi install
```

This works without a Nebi server — `nebi import` pulls directly from the registry.

## What Gets Published

Nebi publishes `pixi.toml` and `pixi.lock` as an OCI artifact with custom media types:

- `application/vnd.pixi.toml.v1+toml`
- `application/vnd.pixi.lock.v1+yaml`

Only Pixi spec files are published. Source code, data, and installed packages are never included.
