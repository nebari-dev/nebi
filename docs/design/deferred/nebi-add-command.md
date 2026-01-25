# Design Proposal: `nebi add` Command

## Status: Deferred (Not MVP)

After analysis, we've determined `nebi add` is not a priority for the MVP. The concrete benefit is narrow — it essentially provides shell activation by name (`nebi shell my-project` from anywhere) without needing to `cd` into the project directory. That's convenience, but not enough to justify a new command and index entry type.

**Why there isn't much benefit:**

1. **No versioning** — without a server/registry involved, there's no version history. It's just a pointer to a directory.
2. **No meaningful tags** — a `tag: "local"` placeholder doesn't serve any purpose. You can't update tags or track changes over time without pushing.
3. **No sharing** — it's purely local bookkeeping that only benefits one user on one machine.
4. **Drift detection is pointless** — for a local project *you're actively developing*, "drift from when you registered it" is just... normal development. You changed it on purpose.
5. **Promotion to push is already trivial** — `nebi push my-project:v1.0` already works from any directory with a `pixi.toml`. You don't need to pre-register to push.
6. **The only real use case is `nebi shell <name>` from anywhere** — but `nebi shell --path ~/projects/my-project` or just `cd ~/projects/my-project && pixi shell` already works fine.

Before push, a workspace is just a pixi project — and pixi already handles local projects well. The value of Nebi comes *after* push, when naming, tagging, versioning, and sharing actually matter.

We may revisit this if users frequently request conda-style "named local environments" (`nebi shell data-science` like `conda activate data-science`), but for now it's not worth the complexity.

---

## Original Proposal (Preserved for Future Reference)

## Problem Statement

Currently, Nebi operates on a push-first model: workspaces only become "known" to Nebi after they are pushed to an OCI registry via `nebi push`. This creates friction in several scenarios:

1. **Offline/disconnected work**: Users can't track workspaces locally without network access to push
2. **Pre-push iteration**: Users want to manage a workspace with Nebi (e.g., use `nebi shell`) before it's ready to share
3. **Git-based sharing**: Teams that share pixi workspaces via git (not OCI registries) can't leverage Nebi's convenience features
4. **Existing projects**: Users with existing pixi projects must push them to a registry just to see them in `nebi workspace list --local`

The fundamental issue is the gap between "I have a pixi project" and "Nebi knows about this workspace."

## Prior Art: Pixi Issue #5277

Pixi is considering a similar feature in [PR #5277](https://github.com/prefix-dev/pixi/issues/5277) with `pixi workspace register`:

```bash
pixi workspace register                           # Register current workspace
pixi workspace register --workspace myproject     # Register with custom name
pixi workspace register list                      # List registered workspaces
pixi workspace register remove <name>             # Remove registration
```

Their approach uses a global registry (`~/.pixi/workspaces`) with symlinks (or fallback files on non-Unix) to track workspace locations by name.

**Key differences for Nebi:**
- Pixi's register is purely local path-to-name mapping
- Nebi uses a local JSON index (`~/.local/share/nebi/index.json`) that also tracks pulled workspaces
- Nebi workspaces can optionally have OCI publication metadata and server-assigned UUIDs

## Relationship to `duplicate-pulls.md`

This proposal extends the **local JSON index** defined in `duplicate-pulls.md`. That document establishes:

- `~/.local/share/nebi/index.json` as the local workspace registry
- Entries with `workspace`, `tag`, `registry`, `path`, `is_global`, `pulled_at`, `manifest_digest`, `layers`
- Drift detection via OCI layer digests
- `.nebi` metadata files per workspace

`nebi add` introduces a new entry type: **locally-added workspaces** that were never pulled from a registry. These coexist in the same index file alongside pulled workspaces.

## Proposed Solution

### Command: `nebi add`

Register a local pixi workspace with Nebi without pushing to a registry.

```bash
# Register current directory with a name
nebi add myworkspace

# Register specific path
nebi add myworkspace --path /path/to/pixi/project

# Register with a local tag (optional, defaults to "local")
nebi add myworkspace --tag dev
```

### Behavior

1. **Validation**: Verify `pixi.toml` exists at the target path (or current directory)
2. **Index registration**: Add entry to `~/.local/share/nebi/index.json` with `source: "local"` (distinguishing it from pulled workspaces)
3. **Metadata file**: Create `.nebi` in the workspace directory with origin info
4. **Compute digests**: Hash `pixi.toml` and `pixi.lock` (if present) for drift detection
5. **Confirmation**: Display the registered workspace info

### Index Entry Format

Added workspaces use the same `index.json` format from `duplicate-pulls.md`, with a `source` field to distinguish them from pulled workspaces:

```jsonc
// ~/.local/share/nebi/index.json
{
  "version": 1,
  "workspaces": [
    // --- Pulled workspace (from duplicate-pulls.md) ---
    {
      "workspace": "data-science",
      "tag": "v1.0",
      "registry": "ds-team",
      "source": "pull",
      "path": "/home/user/project-a",
      "is_global": false,
      "pulled_at": "2024-01-20T10:30:00Z",
      "manifest_digest": "sha256:abc123def456...",
      "layers": {
        "pixi.toml": "sha256:111...",
        "pixi.lock": "sha256:222..."
      }
    },
    // --- Locally added workspace (NEW) ---
    {
      "workspace": "my-ml-project",
      "tag": "local",
      "registry": null,
      "source": "add",
      "path": "/home/user/projects/my-ml-project",
      "is_global": false,
      "added_at": "2024-01-22T09:00:00Z",
      "layers": {
        "pixi.toml": "sha256:555...",
        "pixi.lock": "sha256:666..."
      }
    }
  ]
}
```

**Key differences for `source: "add"` entries:**
- `registry` is `null` (no OCI origin)
- `added_at` replaces `pulled_at`
- No `manifest_digest` (no OCI manifest exists yet)
- `tag` defaults to `"local"` but can be user-specified for organizational purposes
- `layers` contains digests computed locally (sha256 of file content) for drift detection

### The `.nebi` Metadata File

For locally added workspaces, the `.nebi` file records origin differently:

```yaml
# ~/projects/my-ml-project/.nebi
origin:
  workspace: my-ml-project
  tag: local
  source: add           # Distinguishes from "pull"
  added_at: 2024-01-22T09:00:00Z

layers:
  pixi.toml:
    digest: sha256:555...
    size: 1234
  pixi.lock:
    digest: sha256:666...
    size: 34567
```

This uses the same format as pulled workspaces (from `duplicate-pulls.md`) but with `source: add` instead of registry/pull info.

### Example Workflow

```bash
# Create a new pixi project
pixi init my-ml-project
cd my-ml-project
pixi add python numpy pandas

# Register with Nebi (no push required)
nebi add my-ml-project

# Now it appears in workspace list --local
nebi workspace list --local
# LOCAL WORKSPACES
#   WORKSPACE        TAG      STATUS    LOCATION
#   my-ml-project    local    clean     ~/projects/my-ml-project

# Use nebi shell without specifying path
nebi shell my-ml-project

# Evolve the workspace...
pixi add scikit-learn

# Drift is detected (same as pulled workspaces)
nebi workspace list --local
#   WORKSPACE        TAG      STATUS      LOCATION
#   my-ml-project    local    modified    ~/projects/my-ml-project

# Later, when ready to share - push "promotes" it
nebi push my-ml-project:v1.0.0
# → Pushes to default registry
# → Index entry updated: source becomes "push", registry populated
```

## Command Interactions

### `nebi workspace list --local`

Added workspaces appear alongside pulled workspaces, with `source` visible:

```bash
$ nebi workspace list --local
LOCAL WORKSPACES
  WORKSPACE        TAG      SOURCE   STATUS      LOCATION
  data-science     v1.0     pull     clean       ~/project-a
  data-science     v1.0     pull     modified    ~/project-b
  my-ml-project    local    add      clean       ~/projects/my-ml-project
  experiments      dev      add      modified    ~/work/experiments
```

### `nebi push` (Promoting a Local Workspace)

When pushing a workspace that was added locally:

1. Read `pixi.toml` and `pixi.lock` from the registered path
2. Create Environment on the Nebi server (gets UUID assigned)
3. Publish to OCI registry as normal
4. Update `index.json` entry:
   - Set `source` to `"push"` (or keep as `"add"` and add registry info)
   - Populate `registry`, `manifest_digest`
   - Update `layers` with the new OCI layer digests
5. Update `.nebi` file with registry origin info

```bash
# This "promotes" a local workspace to published
$ nebi push my-ml-project:v1.0.0 -r ds-team
Pushing my-ml-project:v1.0.0 to ds-team...
✓ Pushed my-ml-project:v1.0.0

# Index entry is now:
{
  "workspace": "my-ml-project",
  "tag": "v1.0.0",
  "registry": "ds-team",
  "source": "add",
  "path": "/home/user/projects/my-ml-project",
  "is_global": false,
  "added_at": "2024-01-22T09:00:00Z",
  "pushed_at": "2024-01-25T14:00:00Z",
  "manifest_digest": "sha256:abc...",
  "layers": {
    "pixi.toml": "sha256:777...",
    "pixi.lock": "sha256:888..."
  }
}
```

### `nebi pull` (No Change Needed)

Pulled workspaces continue to work exactly as described in `duplicate-pulls.md`. They have `source: "pull"` in the index.

### `nebi shell`

Shell activation behavior matches `duplicate-pulls.md` logic, with added workspaces included in resolution:

```bash
# By name - checks index for matching entries (both "add" and "pull" sources)
$ nebi shell my-ml-project
Activating my-ml-project from ~/projects/my-ml-project...
(my-ml-project) $

# If multiple matches exist (e.g., same name from add + pull)
$ nebi shell data-science
Multiple local copies found for data-science:
  1. ~/projects/data-science (added, tag: local)
  2. ~/project-a (pulled, tag: v1.0)
  3. ~/.local/share/nebi/workspaces/.../v2.0 (global, tag: v2.0)
Enter number or use --path to specify: _
```

### `nebi remove`

The opposite of `nebi add`:

```bash
# Remove from Nebi tracking (does NOT delete files)
nebi remove my-ml-project

# With confirmation prompt
$ nebi remove my-ml-project
This will remove 'my-ml-project' from Nebi tracking.
The pixi files at ~/projects/my-ml-project will NOT be deleted.
Continue? [y/N]: y
✓ Removed my-ml-project from index.

# Force without prompt
nebi remove my-ml-project --force
```

**Behavior:**
- Removes entry from `~/.local/share/nebi/index.json`
- Removes `.nebi` metadata file from workspace directory
- Does NOT delete `pixi.toml`, `pixi.lock`, or `.pixi` directory
- If workspace has been pushed to registries, warn that publications remain in the registry

**What about removing pulled workspaces?** `nebi remove` works for both `source: "add"` and `source: "pull"` entries — it simply removes the index entry and `.nebi` file. The distinction:
- Removing an added workspace: just forgets about it
- Removing a pulled workspace: forgets about it locally, still exists in registry

## Drift Detection (Consistent with `duplicate-pulls.md`)

Added workspaces use the same drift detection as pulled workspaces:

1. At `nebi add` time: compute sha256 of `pixi.toml` and `pixi.lock`, store in `layers`
2. On `workspace list --local`: re-hash local files, compare to stored digests
3. Status values: `clean`, `modified`, `missing` (same as pulled workspaces)

The difference: for pulled workspaces, "drift" means "changed from what was pulled." For added workspaces, "drift" means "changed since registered" (or since last `nebi add --update`).

### Refreshing the Baseline

```bash
# After making intentional local changes, reset the drift baseline:
nebi add my-ml-project --update
# → Re-hashes pixi.toml and pixi.lock
# → Updates layers digests in index.json
# → Status goes back to "clean"
```

## UUID Strategy

**UUIDs are NOT generated on `nebi add`.**

Rationale:
- The JSON index doesn't use UUIDs — it uses `workspace` name + `path` as the unique key
- UUIDs are server-side concepts (assigned when an Environment is created on the Nebi server)
- A UUID is only needed when the workspace is first pushed (`nebi push` creates the server-side Environment record, which gets a UUID via the `BeforeCreate` GORM hook)
- Global storage paths use UUIDs (from `duplicate-pulls.md`), but locally added workspaces stay at their user-specified path — no UUID-based directory needed

This keeps `nebi add` fully offline and independent of the server.

## Alternative Approaches Considered

### Option A: Server Database Storage (Rejected)

Store added workspaces as Environment records in the local Nebi server's SQLite database with new `LocalPath` and `IsLocalOnly` fields.

**Rejected because:**
- `duplicate-pulls.md` established the JSON index as the local workspace registry
- Would create two sources of truth for local workspace tracking
- Requires the local server to be running just to add a workspace
- Over-engineers what is fundamentally a local bookkeeping operation

### Option B: Symlink-Based (Like Pixi's Proposal) (Rejected)

Use filesystem symlinks in a central directory.

**Rejected because:**
- Nebi already has `index.json` for this purpose
- Symlinks are platform-dependent
- Adds complexity without clear benefit over the JSON approach

### Option C: Separate Registry File for Added Workspaces (Rejected)

Create a separate `~/.local/share/nebi/local-workspaces.json` for added workspaces, keeping `index.json` only for pulled workspaces.

**Rejected because:**
- `nebi workspace list --local` and `nebi shell` need to query both sources
- Single file is simpler and already has the `source` field to distinguish entry types
- No benefit to splitting

## Edge Cases

### Path Changes

If user moves the pixi project directory:

```bash
mv ~/projects/my-ml-project ~/work/ml-project
nebi shell my-ml-project  # Fails - path no longer valid
```

**Behavior (matches `duplicate-pulls.md` stale path handling):**
```bash
$ nebi workspace list --local
LOCAL WORKSPACES
  WORKSPACE        TAG      STATUS     LOCATION
  my-ml-project    local    missing    ~/projects/my-ml-project
```

**Solution**: Re-register at new path:
```bash
nebi remove my-ml-project
nebi add my-ml-project --path ~/work/ml-project
```

Or with the `--update` flag:
```bash
nebi add my-ml-project --path ~/work/ml-project --update
# Updates the existing entry's path
```

### Duplicate Names

```bash
nebi add my-project --path ~/work/project-a
nebi add my-project --path ~/work/project-b
```

**Behavior**: Allowed (same as duplicate pulls in `duplicate-pulls.md`). The index can have multiple entries with the same workspace name at different paths. `nebi shell my-project` will prompt for disambiguation if multiple matches exist.

### Name Conflicts with Pulled Workspaces

```bash
# User has pulled data-science:v1.0
nebi add data-science --path ~/my-local-ds
```

**Behavior**: Allowed. These are separate index entries with different paths and different `source` values. The `tag` will distinguish them too (`"local"` vs `"v1.0"`).

### No pixi.lock Present

A freshly-created pixi project may not have `pixi.lock` yet:

```bash
pixi init my-project
nebi add my-project   # No pixi.lock yet
```

**Behavior**: Only hash `pixi.toml`. The `layers` field will not include `pixi.lock`:

```json
{
  "workspace": "my-project",
  "tag": "local",
  "source": "add",
  "path": "/home/user/my-project",
  "layers": {
    "pixi.toml": "sha256:aaa..."
  }
}
```

When `pixi.lock` is later created (via `pixi install`), it will show as "modified" since there's no baseline digest for the lock file. Running `nebi add my-project --update` will refresh the baseline to include it.

## Release Timing Recommendation

**Include in initial release (MVP)**

Rationale:

1. **Essential for offline workflow**: Without `nebi add`, users must push to a registry to use Nebi features
2. **Low complexity**: Straightforward JSON index operation, consistent with `duplicate-pulls.md` patterns
3. **Fills UX gap**: Addresses the "how do I track local workspaces without pushing?" question from design discussions
4. **Natural complement**: `nebi add` is to local workspaces what `nebi pull` is to remote workspaces — both populate the same index

**Suggested implementation order:**
1. `nebi add <name>` — basic registration with index entry + `.nebi` file
2. `nebi remove <name>` — unregistration
3. `nebi push` integration — promoting added workspaces to published
4. `nebi shell` integration — resolving added workspaces by name

## Summary

| Aspect | Recommendation |
|--------|----------------|
| Command name | `nebi add` (and `nebi remove`) |
| Storage | `~/.local/share/nebi/index.json` (same as pulled workspaces) |
| Entry type | `source: "add"` (vs `source: "pull"` for pulled workspaces) |
| UUID timing | No UUID until first `nebi push` (UUIDs are server-side) |
| Drift detection | Same sha256 hashing as pulled workspaces |
| MVP inclusion | Yes |

## Comparison: `nebi add` vs `nebi pull`

| | `nebi add` | `nebi pull` |
|---|---|---|
| **Source** | Local filesystem | OCI registry |
| **Requires network** | No | Yes |
| **Creates index entry** | Yes (`source: "add"`) | Yes (`source: "pull"`) |
| **Creates `.nebi` file** | Yes | Yes |
| **Has OCI manifest digest** | No | Yes |
| **Has registry info** | No | Yes |
| **Drift baseline** | Computed locally at add time | From OCI layer digests |
| **Supports `--global`** | No (workspace stays in place) | Yes |
| **Can be promoted** | Yes, via `nebi push` | Already published |

## Open Questions

1. Should `nebi add` support batch registration?
   ```bash
   nebi add --scan ~/projects/  # Find all pixi.toml files and register?
   ```

2. Should there be a `nebi init` that combines `pixi init` + `nebi add`?
   ```bash
   nebi init my-project  # Creates pixi project AND registers with nebi
   ```

3. Should `nebi add --update` also be available as a separate command (e.g., `nebi refresh`)?

4. When `nebi push` promotes an added workspace, should the index entry's `tag` field update from `"local"` to the pushed tag (e.g., `"v1.0.0"`)?
   - Option A: Update in place (entry now looks like a pull)
   - Option B: Keep the `"local"` tag entry and add a new entry with the pushed tag
   - Option C: Update `tag` but keep `source: "add"` and add `pushed_at` field (recommended, shown above)
