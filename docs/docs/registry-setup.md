---
sidebar_position: 6
---

# Registry Setup

An OCI registry is a server that stores and distributes packages using the [Open Container Initiative](https://opencontainers.org/) standard. Nebi uses OCI registries to publish environment specs (`pixi.toml` and `pixi.lock`) so anyone can import them without needing access to your Nebi server.

Nebi works with any OCI-compliant registry, for example:

- GitHub Container Registry (GHCR)
- Quay.io
- Docker Hub
- Amazon ECR
- Google Artifact Registry
- Azure Container Registry

This page covers setup instructions for the first three.

## GitHub Container Registry (GHCR)

GHCR is the easiest option if you already have a GitHub account. Go to GitHub Settings > Developer settings > Personal access tokens > **Tokens (classic)** and create a new token with the `write:packages` scope checked.

Then add the registry to Nebi:

```bash
nebi registry add \
  --name ghcr \
  --url ghcr.io \
  --namespace your-github-username-or-org \
  --username your-github-username \
  --default
```

The `--namespace` is your username or organization on the registry. It becomes part of the URL: `ghcr.io/<namespace>/<repo-name>`. When prompted for a password, paste the token you created.

:::tip
Public packages on GHCR are free. Anyone can import them with `nebi import ghcr.io/your-username/my-workspace:v1.0`.
:::

## Quay.io

Quay.io is a free container registry by Red Hat. To set it up:

1. Create a [public repository](https://docs.quay.io/guides/create-repo.html)
2. Create a [robot account](https://docs.quay.io/glossary/robot-accounts.html) (e.g., `nebi_push`)
3. In your repository settings, add the robot account with **Write** [permission](https://docs.quay.io/guides/repo-permissions.html)
4. Copy the robot account username and token

Then add the registry to Nebi:

```bash
nebi registry add \
  --name quay \
  --url quay.io \
  --namespace your-quay-username-or-org \
  --username "your-quay-username+nebi_push" \
  --default
```

The username follows the format `username+robot_name`. When prompted for a password, paste the robot account token.

## Docker Hub

Docker Hub is the most widely used container registry. Create a [personal access token](https://docs.docker.com/security/access-tokens/) with **Read & Write** permission.

Then add the registry to Nebi:

```bash
nebi registry add \
  --name dockerhub \
  --url docker.io \
  --namespace your-dockerhub-username-or-org \
  --username your-dockerhub-username \
  --default
```

Replace `your-dockerhub-username` with your Docker Hub username or organization. When prompted for a password, paste the access token.

## Pulling from a Public Registry

You do not need a Nebi server, an account, or registry credentials to consume a public environment. If someone publishes their workspace to a public OCI namespace, you can pull it directly:

```bash
nebi import <registry>/<namespace>/<repo>:<tag>
```

For example:

```bash
nebi import quay.io/nebari_environments/data-science-demo:0.1.0
```

This writes `pixi.toml` and `pixi.lock` into the current directory, ready to run with `pixi run`. To discover public environments visually, see [Browse Public Registries](./ui.md#browse-public-registries).
