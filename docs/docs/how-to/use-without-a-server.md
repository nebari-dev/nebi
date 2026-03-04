---
sidebar_position: 7
---

# Use Nebi Without a Server

Nebi works locally without any server connection. This is useful for personal workspace management or when working with publicly published environments.

## What Works Locally

| Command | Server Required? |
|---------|-----------------|
| `nebi init` | No |
| `nebi workspace list` | No |
| `nebi workspace remove` | No |
| `nebi workspace prune` | No |
| `nebi shell <name>` | No |
| `nebi run <name> <task>` | No |
| `nebi import <oci-ref>` | No |
| `nebi diff ./a ./b` | No (local paths only) |

## Track and Use Workspaces by Name

The core local workflow — track Pixi projects and activate them from anywhere:

```bash
# Track some projects
cd ~/projects/data-science && nebi init
cd ~/projects/ml-pipeline && nebi init

# Use them by name from any directory
nebi shell data-science
nebi run ml-pipeline train
```

This is useful even without a server. Instead of remembering paths or `cd`-ing around, you activate environments by name.

## Import from OCI Registries

Pull published environments without any server:

```bash
nebi import ghcr.io/myorg/data-science:v1.0 -o ./my-env
cd my-env
pixi install
```

The imported workspace is automatically tracked, so you can use it by name afterward.

## Compare Local Directories

Diff works between local paths without a server:

```bash
nebi diff ./project-a ./project-b
```

Server references (`workspace:tag` syntax) require a connection.

## When to Add a Server

Consider connecting to a server when you need to:

- Share environments with teammates
- Version and tag environment specs
- Track who changed what and when
- Publish to OCI registries from the server

See [Set Up a Nebi Server](./set-up-a-server.md) to get started.
