---
sidebar_position: 1
---

# Use Someone Else's Environment

There are two ways to get an environment someone else created: **pull** from a Nebi server, or **import** from an OCI registry.

## Pull from a Nebi Server

If your team runs a Nebi server, ask your teammate for the workspace name and tag, then pull:

```bash
# Log in (one-time)
nebi login https://nebi.company.com

# Browse available workspaces
nebi workspace list --remote

# Pull into a new directory
nebi pull data-science:stable -o ./data-science

# Install the packages
cd data-science
pixi install
```

The `-o` flag specifies the output directory. Without it, Nebi writes to the current directory.

## Import from an OCI Registry

If the environment was published to an OCI registry, you can import it directly — no Nebi server required:

```bash
# Import into a new directory
nebi import ghcr.io/myorg/data-science:v1.0 -o ./data-science

# Install the packages
cd data-science
pixi install
```

The OCI reference format is `registry/repository:tag`. The tag is required.

## After Getting the Environment

Both methods give you a `pixi.toml` and `pixi.lock` in the target directory. The workspace is automatically tracked by Nebi, so you can use it by name:

```bash
nebi shell data-science
nebi run data-science jupyter-lab
```

If you need to update to a newer version later:

```bash
cd data-science

# If you pulled from a server:
nebi pull data-science:v2.0
pixi install

# Or re-pull the same tag (if it was updated):
nebi pull
pixi install
```
