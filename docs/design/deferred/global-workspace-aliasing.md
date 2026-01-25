# Design: Global Workspace Aliasing (Future)

> **Status**: Deferred - not needed for MVP

## Problem Statement

Users may want the same environment content available globally under different names:

```bash
# Pull same content but want two "names" for it
$ nebi pull --global data-science:v1.0 --as my-ds-env
$ nebi pull --global data-science:v1.0 --as project-x-env
```

Currently, global pulls use `workspace:tag` as the identifier, preventing duplicates. This proposal explores adding local aliasing.

## Use Cases

1. **Team conventions**: Your team calls it "ds-env" but registry calls it "data-science"
2. **Multiple contexts**: Same base environment used for different projects, want clear naming
3. **Migration**: Transitioning from one name to another
4. **Experimentation**: Pull same base twice to modify independently

## Proposed Design

### CLI

```bash
# Pull with alias
$ nebi pull --global data-science:v1.0 --as my-ds
✓ Pulled data-science:v1.0 as 'my-ds' (global)

# List shows both origin and alias
$ nebi workspace list --global
GLOBAL WORKSPACES
  ALIAS       ORIGIN               STATUS    LOCATION
  my-ds       data-science:v1.0    clean     ~/.local/share/nebi/workspaces/my-ds/
  proj-env    data-science:v1.0    modified  ~/.local/share/nebi/workspaces/proj-env/

# Shell by alias
$ nebi shell my-ds
Activating my-ds (origin: data-science:v1.0)...
```

### Storage Model

```
~/.local/share/nebi/workspaces/
├── my-ds/                    # Alias is the directory name
│   ├── .nebi                 # Tracks origin: data-science:v1.0
│   ├── pixi.toml
│   └── pixi.lock
└── proj-env/                 # Different alias, same origin
    ├── .nebi                 # Also tracks origin: data-science:v1.0
    ├── pixi.toml
    └── pixi.lock
```

**Note**: These are full copies, not symlinks. Each can drift independently from the origin.

### Index Schema Addition

```sql
CREATE TABLE local_workspaces (
    -- existing fields...

    alias TEXT,                     -- Local name (for global workspaces with --as)
    -- alias is NULL for directory pulls and non-aliased global pulls

    UNIQUE(alias) WHERE alias IS NOT NULL AND is_global = TRUE
);
```

### Behavior Rules

| Scenario | Behavior |
|----------|----------|
| `nebi pull --global ws:tag` | Uses `ws` as directory name, no alias |
| `nebi pull --global ws:tag --as foo` | Uses `foo` as directory name, stores alias |
| `nebi pull --global ws:tag --as foo` (alias exists) | Error: "Alias 'foo' already exists" |
| `nebi shell foo` | Resolves alias to workspace, activates |
| `nebi shell ws:tag` | Finds by origin (may match multiple aliases) |

### Conflict Resolution

When `nebi shell ws:tag` matches multiple aliased copies:

```bash
$ nebi shell data-science:v1.0
Multiple global copies found for data-science:v1.0:
  1. my-ds (clean)
  2. proj-env (modified)
Enter number or use alias directly: _
```

## Workaround (Current)

Until this feature is implemented, users can achieve similar results with `--path`:

```bash
$ nebi pull data-science:v1.0 --path ~/.local/share/nebi/workspaces/my-custom-name/
```

This works but:
- Won't show up in `nebi workspace list --global` (it's a directory pull)
- No alias tracking in the index
- User must remember the custom path

## Implementation Complexity

**Low**:
- Add `--as` flag to pull command
- Add `alias` column to index
- Update list/shell to handle aliases

**Medium**:
- Alias uniqueness validation
- Disambiguation when multiple aliases share an origin
- Clear UX for alias vs origin in all commands

## Decision

Defer to post-MVP. The workaround with `--path` is sufficient for early users, and we can gauge demand before adding complexity.

## Related

- [duplicate-pulls.md](duplicate-pulls.md) - Main duplicate handling design
