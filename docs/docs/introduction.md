---
sidebar_position: 1
---

# Introduction

Nebi is a CLI tool that adds version control, team sharing, and governance to [Pixi](https://pixi.sh) environments. Think of it as **git for environments**.

## The Problem

If you use pip or uv, you've likely hit this wall: your project needs GDAL, CUDA, or another compiled C/C++ library, and `pip install` fails because the system dependency isn't there.

```text
× Failed to build `gdal==3.12.2`
╰─▶ gdal_config_error: No such file or directory: 'gdal-config'
    Could not find gdal-config. Make sure you have installed
    the GDAL native library and development headers.
```

Fixing this means installing system libraries through your OS package manager, matching exact versions, and hoping your teammate's machine is configured the same way. Every new team member repeats this process from scratch.

**Pixi** solves the dependency problem — it manages both Python packages (from PyPI) and compiled system libraries (from conda-forge) in a single lockfile, so `pixi add gdal` just works across platforms.

But pixi wasn't designed for **team environment management**:

- **No version history** — the lockfile is overwritten on every change, with no way to diff or roll back
- **No sharing** — environments are tied to project directories, with no way to publish or distribute them
- **No governance** — anyone who can edit `pixi.toml` can change the environment, with no approval workflows or access control

Nebi fills these gaps.

## Why Nebi?

### Version and roll back environments

Push snapshots as you iterate, compare what changed between versions, and roll back when an update breaks your workflow:

```bash
nebi push geo-ml:v1.0

# Later, after adding packages...
nebi push geo-ml:v2.0
nebi diff geo-ml:v1.0 geo-ml:v2.0

# Something broke? Roll back.
nebi pull geo-ml:v1.0
pixi install
```

### Share environments across your team

Publish environments to an OCI registry (the same standard behind Docker Hub) so anyone can reproduce your setup without cloning your repo:

```bash
nebi publish my-project --tag v1.0.0
# A colleague on another machine:
nebi import quay.io/nebari/data-science:v1.0 -o ./my-project
```

### Control who can modify production dependencies

Role-based access control and OIDC authentication let you enforce who can push, pull, or modify shared environments — so a Friday afternoon `pixi add` doesn't silently break Monday's production pipeline.

### Use workspaces by name from anywhere

No need to `cd` into a project directory. Activate any tracked workspace by name:

```bash
nebi shell geo-ml    # from any directory
nebi run geo-ml train
```

## How Does It Compare?

| Feature | pip/uv | conda | pixi | nebi |
| --- | --- | --- | --- | --- |
| Compiled system libraries (GDAL, CUDA) | No | Yes | Yes | Yes |
| Lockfiles with exact versions | uv only | No | Yes | Yes |
| Fast dependency resolution | uv only | No | Yes | Yes |
| PyPI + conda-forge in one tool | No | Limited | Yes | Yes |
| Environment versioning and diffing | No | No | No | Yes |
| Team sharing via registries | No | No | No | Yes |
| Role-based access control | No | No | No | Yes |

## Next Steps

- [Getting Started](./getting-started.md) — Install nebi and set up your first workspace
- [Architecture](./architecture.md) — How the CLI, server, and OCI registries fit together
- [CLI Guide](./cli-guide.md) — Local workflows and team collaboration
