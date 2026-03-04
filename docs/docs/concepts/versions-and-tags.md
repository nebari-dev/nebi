---
sidebar_position: 3
---

# Versions and Tags

When you push a workspace to a Nebi server, the server creates **versions** and **tags** to track the history of your environment specs.

## Versions

A version is an immutable snapshot of your `pixi.toml` and `pixi.lock` at a point in time. Each push with new content creates a new version with an auto-incrementing number (1, 2, 3, ...).

Versions are permanent — once created, a version's content never changes.

## Tags

Tags are human-readable names that point to specific versions. They come in two flavors:

### Automatic Tags

Every push creates two automatic tags:

- **`sha-<hash>`** — A content-addressed tag computed from the `pixi.toml` and `pixi.lock` contents. Identical content always produces the same hash, regardless of when it was pushed. This is the most reliable way to reference a specific environment state.

- **`latest`** — Always points to the most recently pushed version. Updated on every push.

### User Tags

You can add your own tags when pushing:

```bash
nebi push my-project:production
```

User tags are mutable — you can move them to point to a different version by pushing again with the same tag name.

## Content-Addressed Deduplication

Nebi uses content hashing to avoid duplicate versions. If you push content identical to an existing version, no new version is created. Instead, Nebi reuses the existing version and attaches any new tags to it.

This means:

- Pushing the same `pixi.toml` and `pixi.lock` twice doesn't create version 1 and version 2 — it creates version 1 and tells you the content was unchanged.
- The `sha-<hash>` tag is stable: the same content always maps to the same hash.
- You can safely push multiple tags for the same content without wasting storage.

## How Tags, Versions, and Content Relate

```
Tags           Versions        Content
────           ────────        ───────
latest    ──→  version 2  ──→  pixi.toml (v2) + pixi.lock (v2)
stable    ──→  version 2
sha-e4f5  ──→  version 2

v1.0      ──→  version 1  ──→  pixi.toml (v1) + pixi.lock (v1)
sha-a1b2  ──→  version 1
```

Multiple tags can point to the same version. A version's content is fixed once created.

## Using Tags in Commands

The `workspace:tag` syntax is used throughout Nebi:

```bash
nebi pull my-project:v1.0          # Pull a specific tagged version
nebi diff my-project:v1.0 my-project:latest  # Compare two versions
```

Omitting the tag defaults to the latest version.
