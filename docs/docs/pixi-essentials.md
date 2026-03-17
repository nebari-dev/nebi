---
sidebar_position: 2
---

# Pixi Basics

Nebi manages environments built with [Pixi](https://pixi.sh), a cross-platform package manager built on the conda ecosystem. This page covers the Pixi commands you'll use most often when working with Nebi workspaces.

> If you're already comfortable with Pixi, you can skip ahead to the [CLI Guide](./cli-guide.md).

## Install Pixi

```bash
curl -fsSL https://pixi.sh/install.sh | bash
```

See the [Pixi installation docs](https://pixi.sh) for more options.

## Add Dependencies

Use `pixi add` to add packages to your workspace. This updates `pixi.toml` and `pixi.lock` automatically:

```bash
# Add a conda package
pixi add numpy

# Add a specific version
pixi add "python=3.12.*"

# Add a PyPI package
pixi add --pypi flask

# Add a PyPI package with extras and version pin
pixi add --pypi "flask[async]==3.1.0"
```

## Remove Dependencies

```bash
pixi remove numpy
```

This removes the package from `pixi.toml` and updates `pixi.lock`.

## Install the Environment

After pulling a workspace or editing `pixi.toml` manually, run:

```bash
pixi install
```

This resolves dependencies and installs all packages defined in the workspace.

## List Installed Packages

```bash
pixi list
```

## Run Commands and Tasks

Run a command or a task defined in `pixi.toml` inside the workspace environment:

```bash
# Run a defined task
pixi run test

# Run an arbitrary command
pixi run python my_script.py
```

## Activate a Shell

Open an interactive shell with the workspace environment activated:

```bash
pixi shell
```

## Typical Nebi + Pixi Workflow

Here's how Pixi commands fit into a typical Nebi workflow:

```bash
# 1. Create and track a new workspace
mkdir my-project && cd my-project
nebi init

# 2. Add dependencies with Pixi
pixi add numpy pandas matplotlib

# 3. Work in the environment
pixi shell

# 4. Push the workspace to your team
nebi push my-project
```

## Learn More

For the full Pixi documentation, visit [pixi.sh](https://pixi.sh).
