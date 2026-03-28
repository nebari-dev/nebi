---
sidebar_position: 6
---

# Registry Setup

Nebi can publish environments to OCI registries so anyone can import them without needing access to your Nebi server. This page covers how to set up two popular registries.

## GitHub Container Registry (GHCR)

GHCR is the easiest option if you already have a GitHub account. Go to GitHub Settings > Developer settings > Personal access tokens and create a token with the `write:packages` scope.

Then add the registry to Nebi:

```bash
nebi registry add \
  --name ghcr \
  --url ghcr.io \
  --namespace your-github-username \
  --username your-github-username \
  --default
```

Replace `your-github-username` with your GitHub username. When prompted for a password, paste the token you created.

:::tip
Public packages on GHCR are free. Anyone can import them with `nebi import oci://ghcr.io/your-username/my-workspace:v1.0`.
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
  --namespace your-quay-username \
  --username "your-quay-username+nebi_push" \
  --default
```

The username follows the format `username+robot_name`. When prompted for a password, paste the robot account token.
