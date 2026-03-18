---
sidebar_position: 1
---

# Introduction

Nebi is a multi-user environment management tool. Think of it as git for environments: you can push, pull, diff, and roll back, with access control built in.

## The Problem

If you use pip or uv, you might run into this: your project needs GDAL, CUDA, or another compiled C/C++ library, and `pip install` fails because the system dependency isn't there.

```text
× Failed to build `gdal==3.12.2`
╰─▶ gdal_config_error: No such file or directory: 'gdal-config'
    Could not find gdal-config. Make sure you have installed
    the GDAL native library and development headers.
```

These are system-level dependencies that pip and uv cannot install. Fixing this means falling back to your OS package manager, matching exact versions, and repeating the process on every machine.

**[Pixi](https://pixi.sh)** solves the dependency problem by managing both Python packages (from PyPI) and compiled system libraries (from conda-forge) in a single lockfile, so `pixi add gdal` just works across platforms.

But pixi wasn't designed for **team environment management**:

- **No version history:** The lockfile is overwritten on every change, with no way to diff or roll back.
- **No sharing:** Environments are tied to project directories, with no way to publish or distribute them.
- **No governance:** Anyone who can edit `pixi.toml` can change the environment, with no approval workflows or access control.

Nebi fills these gaps.

## Why Nebi?

Since nebi builds on pixi, you get all of pixi's features plus team collaboration on top.

### Install system libraries alongside Python packages

With nebi, `pixi add gdal` installs both the Python bindings and the compiled C/C++ library. No separate `apt-get` or `brew` steps needed:

```bash
pixi add gdal geopandas lightgbm
pixi add --pypi scikit-learn
```

### Reproducible environments with lockfiles

Without a lockfile, the same install can produce different environments on different machines or at different times. Pixi solves this by generating a lockfile on every `pixi add`, pinning every transitive dependency to an exact version and hash:

```bash
pixi add geopandas gdal lightgbm
```

This generates `pixi.toml` and `pixi.lock`, which together pin every transitive dependency to an exact version, hash, and download URL.

Once you commit both files to version control, a teammate can pull and recreate the exact same environment:

```bash
# Pull the latest pixi.toml and pixi.lock
git pull

# Recreate the exact same environment
pixi install
```

### Built-in task runner

Data science projects involve commands that are hard to remember, like `python src/train.py --config configs/experiment_3.yaml --epochs 100`.

Pixi has a built-in task runner that stores these commands alongside your dependencies, so anyone on the team can run them without memorizing the details:

```bash
# Define tasks
pixi task add train "python src/train.py"
pixi task add test "pytest tests/"

# Run by name
pixi run train
```

### Version and roll back environments

When a dependency update breaks your pipeline, you need to know exactly what changed. Diffing a 500-line lockfile in git history isn't practical.

Nebi lets you push snapshots, compare what changed between versions, and roll back when an update breaks your workflow:

```bash
# Push a snapshot of your current environment
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
# Publish to an OCI registry
nebi publish my-project --tag v1.0.0

# A colleague on another machine:
nebi import quay.io/nebari/data-science:v1.0 -o ./my-project
```

### Control who can modify production dependencies

Role-based access control and OIDC authentication let you enforce who can push, pull, or modify shared environments. Every change is tracked, so you always know who modified what and when.

### Use workspaces by name from anywhere

In data science, it's common to reuse the same environment across multiple projects. With both uv and pixi, environments are tied to project directories, so you'd have to `cd` into the source project each time:

```bash
cd ~/projects/geo-ml && pixi shell   # must cd into the source project
cd ~/projects/analysis && pixi shell # must cd back again for a different project
```

Nebi lets you activate any tracked workspace by name from anywhere:

```bash
cd ~/projects/analysis
nebi shell geo-ml       # no need to cd back

cd ~/projects/dashboard
nebi shell geo-ml       # same environment, any directory
```

## How Does It Compare?

| Feature | uv | conda | pixi | nebi |
| --- | --- | --- | --- | --- |
| Compiled system libraries (GDAL, CUDA) | No | Yes | Yes | Yes |
| Lockfiles with exact versions | Yes | No | Yes | Yes |
| Fast dependency resolution | Yes | No | Yes | Yes |
| PyPI + conda-forge in one tool | No | Limited | Yes | Yes |
| Environment versioning and diffing | No | No | No | Yes |
| Team sharing via registries | No | No | No | Yes |
| Role-based access control | No | No | No | Yes |

## Next Steps

- [Getting Started](./getting-started.md): Install nebi and set up your first workspace
- [Architecture](./architecture.md): How the CLI, server, and OCI registries fit together
- [CLI Guide](./cli-guide.md): Local workflows and team collaboration
