---
sidebar_position: 0
---

# Why Nebi

## The Problem

[Pixi](https://pixi.sh) is excellent at creating reproducible environments on a single machine. You define your dependencies in `pixi.toml`, Pixi solves and locks them, and you get a deterministic environment every time.

But what happens when you need to share that environment with your team? Or reproduce it on another machine? Or track how it changed over time?

Today, teams typically resort to:

- Committing `pixi.toml` and `pixi.lock` into a Git repo alongside source code
- Copying files over Slack or email
- Writing internal wiki pages with "run these commands to set up your environment"

These approaches are fragile. Environments drift. People forget to update the shared copy. There's no way to diff what changed between versions, or roll back to a known-good state.

## What Nebi Does

Nebi adds collaboration and versioning on top of Pixi. Think of it like this:

> **Pixi** creates and solves environments. **Nebi** tracks, versions, and shares them.

If you're familiar with Git, the analogy is straightforward:

| Git (for code) | Nebi (for environments) |
|---|---|
| `git init` | `nebi init` |
| `git push` | `nebi push` |
| `git pull` | `nebi pull` |
| `git diff` | `nebi diff` |
| `git status` | `nebi status` |

Nebi doesn't replace Pixi — it extends it. Every Nebi workspace is a standard Pixi project with a `pixi.toml` and `pixi.lock`. Nebi just adds the ability to push, pull, version, and publish those specs.

## Who is Nebi For?

- **Data science teams** who need everyone running the same environment
- **Platform engineers** who publish curated environments for their organization
- **Researchers** who want to share reproducible computational environments
- **Anyone using Pixi** who works with more than one machine or one person

## What Nebi is Not

- **Not a package manager.** Nebi doesn't install packages — Pixi does. Nebi manages the specs (`pixi.toml` and `pixi.lock`) that tell Pixi what to install.
- **Not a replacement for Pixi.** You still use Pixi directly for adding dependencies, creating environments, and running tasks. Nebi wraps `pixi shell` and `pixi run` for convenience, but the heavy lifting is all Pixi.
- **Not a replacement for Git.** Nebi versions environment specs, not source code. Use both together.

## How it Works

Nebi has three layers, and you only use the ones you need:

**Local** — Track Pixi workspaces on your machine. Use `nebi shell` and `nebi run` to activate environments by name from any directory. No server required.

**Server** — Push and pull versioned environment specs to a shared Nebi server. Your team can browse, pull, and diff environments. The server handles access control and versioning.

**Published** — Publish environment specs to OCI registries (like GitHub Container Registry or Quay.io) for broad distribution. Anyone can import a published environment without needing access to your server.

You can start local-only and add a server later. Or skip the server entirely and publish directly to an OCI registry using `nebi import`.
