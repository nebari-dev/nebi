---
sidebar_position: 4
---

# Versions and Tags

Every time you push to the server, Nebi creates a version. Tags are human-friendly names that point to specific versions. Understanding how they work helps you manage your environments effectively.

## Versions

Each push with new content creates a new **version** with an auto-incrementing number:

```bash
nebi push my-project
# Pushed my-project (version 1, tags: sha-a1b2c3d, latest)

# ... modify pixi.toml ...

nebi push my-project
# Pushed my-project (version 2, tags: sha-e4f5g6h, latest)
```

Versions are immutable — once created, a version's content never changes.

## Automatic Tags

Every push creates two automatic tags:

- **`sha-<hash>`** — A content-addressed tag computed from the `pixi.toml` and `pixi.lock` contents. Two pushes with identical content always produce the same hash. This is the most precise way to reference a version.
- **`latest`** — Always points to the most recently pushed version. Updated on every push.

## User Tags

You can add your own tags when pushing:

```bash
nebi push my-project:production
```

Multiple tags can point to the same version. For example, after this push, version 1 might have tags: `sha-a1b2c3d`, `latest`, and `production`.

You can move a tag to a different version by pushing again with the same tag:

```bash
# Point 'production' to the new version
nebi push my-project:production
```

Use `--force` if you need to reassign a tag that already points to a different version of the same content.

## Content-Addressed Deduplication

If you push content that's identical to an existing version, Nebi doesn't create a new version. Instead, it reuses the existing one and attaches any new tags:

```bash
# First push
nebi push my-project:v1.0
# Pushed my-project (version 1, tags: sha-a1b2c3d, latest, v1.0)

# Push again without changes
nebi push my-project:stable
# Content unchanged — my-project (version 1, tags: sha-a1b2c3d, latest, v1.0, stable)
```

This means `sha-<hash>` tags are stable identifiers — the same content always maps to the same hash, regardless of when or how many times it's pushed.

## Viewing Tags

List all tags for a workspace:

```bash
nebi workspace tags my-project
```

```
TAG               VERSION  CREATED           UPDATED
production        2        2024-01-15 14:22
latest            2        2024-01-15 10:30  2024-01-15 14:22
v1.0              1        2024-01-15 10:30
sha-b2c3d4e5f6a7  2        2024-01-15 14:22
sha-a1b2c3d4e5f6  1        2024-01-15 10:30
```

## Referencing Versions

Throughout Nebi, you reference server workspaces using the `workspace:tag` syntax:

```bash
nebi pull my-project:v1.0
nebi diff my-project:v1.0 my-project:production
nebi publish my-project --tag v1.0
```

If you omit the tag, Nebi uses the latest version.

## Next Steps

Now that you understand versioning, let's look at how to [check status and diff environments](./status-and-diffing.md).
