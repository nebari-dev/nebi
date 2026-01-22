# Design: Handling Duplicate Workspace Pulls

## Problem Statement

When a user pulls the same `workspace:tag` to multiple directories:

```bash
cd ~/project-a && nebi pull data-science:v1.0
cd ~/project-b && nebi pull data-science:v1.0
```

Several questions arise:
1. Should this be allowed?
2. How should the local index handle multiple entries for the same `workspace:tag`?
3. Should we warn the user or offer alternatives (e.g., symlinks)?
4. How should `--global` pulls behave regarding duplicates?

## Context

Nebi supports two pull workflows:

1. **Pixi-native (default)**: `nebi pull <workspace>:<tag>` writes to current directory
2. **Conda-like (`--global`)**: `nebi pull --global <workspace>:<tag>` writes to `~/.local/share/nebi/workspaces/`

Each pulled workspace gets a `.nebi` metadata file and is registered in a local index (`~/.local/share/nebi/index.json`).

## How Other Tools Handle This

| Tool | Duplicates Allowed? | Deduplication Strategy | User Warning |
|------|---------------------|----------------------|--------------|
| **pnpm** | No (by design) | Central store + hardlinks | N/A - prevents via architecture |
| **Go modules** | No (global cache) | `$GOPATH/pkg/mod` single copy | N/A - automatic |
| **Nix** | No (content-addressed) | `/nix/store` with hardlinks | N/A - automatic |
| **npm/yarn** | Yes | Full copies; manual `dedupe` command | No |
| **Cargo** | Yes (across workspaces) | Lock file ensures version consistency | `cargo tree --duplicates` |
| **conda/mamba** | Yes (per environment) | Shared package cache (optional) | No |
| **Docker** | Yes (per node) | Layer-based caching | No |
| **git** | Yes (separate clones) | N/A | No |

**Key insight**: Tools that prioritize disk efficiency use content-addressed central caches (pnpm, Go, Nix). Tools that prioritize workflow flexibility allow duplicates (npm, conda, Docker).

## Options Considered

### Option A: Allow Duplicates, Track All Locations

**Behavior**: Multiple pulls of the same `workspace:tag` to different directories are allowed. The local index tracks all locations.

```
Local Index (nebi.db):
┌────────────────────┬──────────────┬─────────────────────────┐
│ workspace:tag      │ registry     │ path                    │
├────────────────────┼──────────────┼─────────────────────────┤
│ data-science:v1.0  │ ds-team      │ /home/user/project-a    │
│ data-science:v1.0  │ ds-team      │ /home/user/project-b    │
│ data-science:v1.0  │ ds-team      │ ~/.local/share/nebi/... │
└────────────────────┴──────────────┴─────────────────────────┘
```

**CLI behavior**:
```bash
# Both pulls succeed, both tracked
$ cd ~/project-a && nebi pull data-science:v1.0
Pulled data-science:v1.0 → ~/project-a/

$ cd ~/project-b && nebi pull data-science:v1.0
Pulled data-science:v1.0 → ~/project-b/

# nebi shell requires disambiguation when multiple local copies exist
$ nebi shell data-science:v1.0
Multiple local copies found:
  1. ~/project-a (pulled 2 days ago)
  2. ~/project-b (pulled 1 hour ago)
  3. ~/.local/share/nebi/workspaces/data-science/v1.0 (--global)
Use --path to specify, or enter number: _
```

**Pros**:
- Matches pixi/git mental model (workspaces are independent)
- No restrictions on user workflows
- Supports legitimate use cases (isolated experiments, different local modifications)
- Simple to implement

**Cons**:
- Disk space inefficiency
- Index can grow large with many duplicates
- `nebi shell <workspace>` needs disambiguation logic
- No automatic deduplication

---

### Option B: Warn on Duplicate, Offer Symlink

**Behavior**: Detect when pulling same `workspace:tag` that already exists locally. Warn and offer to create a symlink instead.

```bash
$ cd ~/project-a && nebi pull data-science:v1.0
Pulled data-science:v1.0 → ~/project-a/

$ cd ~/project-b && nebi pull data-science:v1.0
⚠ data-science:v1.0 already exists at ~/project-a/

Options:
  1. Pull anyway (creates duplicate)
  2. Symlink to existing copy
  3. Cancel

Choice [1/2/3]: _
```

**With flags**:
```bash
# Force duplicate
$ nebi pull data-science:v1.0 --force

# Symlink to existing
$ nebi pull data-science:v1.0 --link
Created symlink: ~/project-b → ~/project-a

# Non-interactive default (e.g., CI): pull anyway
$ nebi pull data-science:v1.0 --yes
```

**Pros**:
- User awareness of duplicates
- Disk space savings when user chooses symlink
- Flexibility preserved (can still duplicate if needed)
- Good UX for interactive use

**Cons**:
- Symlinks can break if source is moved/deleted
- Complexity in tracking symlinked vs real copies
- Symlinks don't work well across filesystems
- Extra cognitive load for users

---

### Option C: Central Cache with Hardlinks (pnpm model)

**Behavior**: All workspace content stored once in `~/.local/share/nebi/cache/`. Directory pulls create hardlinks (or copies on different filesystems).

```
~/.local/share/nebi/
├── cache/                          # Content-addressed store
│   └── sha256-abc123.../
│       ├── pixi.toml
│       └── pixi.lock
└── workspaces/                     # Index only (for --global)
    └── data-science/
        └── v1.0 → ../../cache/sha256-abc123...

~/project-a/
├── pixi.toml  ──[hardlink]──→ ~/.local/share/nebi/cache/sha256.../pixi.toml
└── pixi.lock  ──[hardlink]──→ ~/.local/share/nebi/cache/sha256.../pixi.lock
```

**Pros**:
- Maximum disk efficiency
- Automatic deduplication
- Fast pulls after first download (just create links)

**Cons**:
- Hardlinks don't work across filesystems
- Editing a file in one project silently affects all copies (hardlink behavior)
- Complex implementation
- Unfamiliar behavior for most users
- Pixi may not handle hardlinked manifests well

---

### Option D: Prevent Duplicates for `--global`, Allow for Directory Pulls

**Behavior**: Hybrid approach based on workflow intent:
- **Directory pulls (default)**: Always allowed, even if duplicate. Each directory is treated as independent (pixi-native workflow).
- **Global pulls (`--global`)**: Only one copy per `workspace:tag`. Re-pulling updates the existing copy.

```bash
# Directory pulls - always succeed
$ cd ~/project-a && nebi pull data-science:v1.0
Pulled data-science:v1.0 → ~/project-a/

$ cd ~/project-b && nebi pull data-science:v1.0
Pulled data-science:v1.0 → ~/project-b/  # Duplicate OK

# Global pulls - single copy
$ nebi pull --global data-science:v1.0
Pulled data-science:v1.0 → ~/.local/share/nebi/workspaces/data-science/v1.0/

$ nebi pull --global data-science:v1.0
data-science:v1.0 already exists at ~/.local/share/nebi/workspaces/data-science/v1.0/
Use --force to re-pull and overwrite.
```

**Index behavior**:
```
Local Index tracks:
- Directory pulls: path, workspace:tag, registry, pulled_at
- Global pulls: workspace:tag, registry, pulled_at (path is deterministic)
```

**Pros**:
- Matches user intent: directory pulls are project-specific; global pulls are shared resources
- Clean semantics for `--global` (single source of truth)
- Allows experimentation in project directories
- Simple mental model

**Cons**:
- Two different behaviors to understand
- Directory duplicates still waste disk space
- No deduplication for directory pulls

---

## Recommendation: Option D (Hybrid Approach)

**Rationale**:

1. **Workflow alignment**: The two pull modes represent fundamentally different intents:
   - Directory pulls → "I want this workspace in my project" (isolation-focused)
   - Global pulls → "I want this workspace available system-wide" (sharing-focused)

2. **Pixi-native compatibility**: Directory pulls behave exactly like `pixi init` - you can have the same `pixi.toml` in many directories. This is expected behavior for pixi users.

3. **Conda-like UX for global**: Users expecting `conda activate myenv` semantics get predictable behavior with `--global`. There's one copy of `data-science:v1.0`, not scattered duplicates.

4. **Simplicity**: No complex deduplication, no symlinks to manage, no breaking hardlink behavior.

5. **Industry precedent**: This matches how `go mod` (global cache) and `npm` (project copies) coexist in the Go ecosystem.

## CLI Behavior Examples

### Directory Pulls (Default)

```bash
# First pull
$ cd ~/project-a && nebi pull data-science:v1.0 -r ds-team
Pulling data-science:v1.0 from ds-team...
✓ Pulled to ~/project-a/
  pixi.toml: 2.3 KB
  pixi.lock: 45.2 KB

# Duplicate pull to different directory (allowed, no warning)
$ cd ~/project-b && nebi pull data-science:v1.0 -r ds-team
Pulling data-science:v1.0 from ds-team...
✓ Pulled to ~/project-b/
  pixi.toml: 2.3 KB
  pixi.lock: 45.2 KB

# Re-pull to same directory (overwrites)
$ cd ~/project-a && nebi pull data-science:v1.0 -r ds-team
Pulling data-science:v1.0 from ds-team...
✓ Updated ~/project-a/ (was already at v1.0, re-pulled)

# Pull different tag to same directory
$ cd ~/project-a && nebi pull data-science:v2.0 -r ds-team
⚠ ~/project-a/ already contains data-science:v1.0
Overwrite with data-science:v2.0? [y/N]: y
✓ Pulled to ~/project-a/
```

### Global Pulls

```bash
# First global pull
$ nebi pull --global data-science:v1.0 -r ds-team
Pulling data-science:v1.0 from ds-team...
✓ Pulled to ~/.local/share/nebi/workspaces/data-science/v1.0/

# Duplicate global pull (blocked by default)
$ nebi pull --global data-science:v1.0 -r ds-team
data-science:v1.0 already exists globally.
  Location: ~/.local/share/nebi/workspaces/data-science/v1.0/
  Pulled: 2 hours ago
Use --force to re-pull and overwrite.

# Force re-pull
$ nebi pull --global data-science:v1.0 -r ds-team --force
Re-pulling data-science:v1.0...
✓ Updated ~/.local/share/nebi/workspaces/data-science/v1.0/

# Different tag (separate directory, allowed)
$ nebi pull --global data-science:v2.0 -r ds-team
Pulling data-science:v2.0 from ds-team...
✓ Pulled to ~/.local/share/nebi/workspaces/data-science/v2.0/
```

### Shell Activation

```bash
# From project directory (uses local copy)
$ cd ~/project-a && nebi shell
Activating data-science:v1.0...
(data-science) $

# By name (prefers global, falls back to most recent local)
$ nebi shell data-science:v1.0
Activating data-science:v1.0 from ~/.local/share/nebi/workspaces/...
(data-science) $

# By name when only local copies exist
$ nebi shell data-science:v1.0
Found local copy at ~/project-a/ (pulled 2 hours ago)
Activating...
(data-science) $

# By name when multiple local copies exist (no global)
$ nebi shell data-science:v1.0
Multiple local copies found for data-science:v1.0:
  1. ~/project-a (pulled 2 days ago)
  2. ~/project-b (pulled 1 hour ago)
Enter number or use --path to specify: _
```

### Workspace List

```bash
$ nebi workspace list --local
LOCAL WORKSPACES
────────────────────────────────────────────────────────────
  WORKSPACE           TAG     LOCATION
────────────────────────────────────────────────────────────
  data-science        v1.0    ~/project-a (local)
  data-science        v1.0    ~/project-b (local)
  data-science        v1.0    ~/.local/share/nebi/... (global)
  data-science        v2.0    ~/.local/share/nebi/... (global)
  ml-tools            latest  ~/ml-project (local)
────────────────────────────────────────────────────────────
```

## Local Index Format

The local index is a JSON file. See "Local Index Format" section below for the full schema.

## Edge Cases

### Path Moved/Deleted

The index can become stale if users move or delete directories outside of nebi.

**Solution**: Lazy verification. When listing or activating, check if path exists:
- If exists: use it
- If missing: mark as stale, optionally prompt to re-pull or remove from index

```bash
$ nebi workspace list --local
LOCAL WORKSPACES
  WORKSPACE           TAG     LOCATION
  data-science        v1.0    ~/project-a (local)
  data-science        v1.0    ~/project-b (missing!)  ← detected on list
  ml-tools            latest  ~/.local/share/nebi/... (global)

Run 'nebi workspace prune' to remove stale entries.
```

### Different Content, Same Tag (Registry-Side Drift)

If a registry is updated (tag moved to new content), local copies become outdated.

**Solution**: Store digests and let the user explicitly check with `nebi status --remote`:
```bash
$ nebi status --remote
⚠ Tag 'v1.0' now points to sha256:xyz... (was sha256:abc... when pulled)
  The tag has been updated since you pulled.

  nebi diff --remote     # See what changed
  nebi pull --force      # Get latest
```

Note: `nebi shell` does NOT implicitly check the registry. Network access is always
explicit via `--remote` flags. This keeps shell activation fast and offline-capable.
See `diff-workflow.md` "Why Not `nebi fetch`?" for rationale.

### Cross-Filesystem Global Storage

If `~/.local/share/nebi/` is on a different filesystem than the user's projects, hardlinks won't work (if we ever add them).

**Solution**: For now, full copies everywhere. Hardlink optimization can be added later as an opt-in feature for same-filesystem scenarios.

---

## Local Environment Drift (User Modifications)

### The Problem

Users will evolve their environments over time. After pulling `data-science:v1.0`, a user might:
- Add packages via `pixi add numpy`
- Modify `pixi.toml` directly
- Run `pixi update` to refresh the lock file

Now the local state has **drifted** from the originally pulled `data-science:v1.0`. When we list workspaces, what should we show?

```
Current approach shows:
  WORKSPACE           TAG     LOCATION
  data-science        v1.0    ~/project-a (local)

But ~/project-a/pixi.toml no longer matches what v1.0 was!
```

This creates confusion:
- User thinks they have `v1.0` but actually have a modified version
- `nebi shell data-science:v1.0` might activate a workspace that doesn't match the registry's `v1.0`
- Pushing this back would create a new version that differs from `v1.0`

### Drift Detection Strategies

#### Strategy 1: OCI Layer Digests (Recommended)

Nebi's OCI publisher (`internal/oci/publisher.go`) stores `pixi.toml` and `pixi.lock` as raw blob layers (not tar archives), so the OCI layer digest for each file **is already a pure content hash** (sha256 of file bytes). No filesystem metadata (mtime, permissions, uid/gid) is included.

This means we can reuse the OCI layer digests directly for drift detection - we get them for free during pull and they're already stored in the OCI manifest.

**Implementation**: At pull time, record the per-file layer digests from the OCI manifest. On list/activate, re-hash the local files and compare.

```bash
$ nebi workspace list --local
LOCAL WORKSPACES
  WORKSPACE           TAG       STATUS      LOCATION
  data-science        v1.0      modified    ~/project-a (local)
  data-science        v1.0      clean       ~/project-b (local)
  ml-tools            latest    clean       ~/.local/share/nebi/... (global)
```

**Status values**:
- `clean`: Local file hashes match the pulled layer digests
- `modified`: Local file hashes differ from pulled layer digests
- `missing`: Path no longer exists
- `unknown`: Can't determine (e.g., `.nebi` metadata missing)

**Note on "substantive" vs cosmetic changes**: This approach detects any byte-level change, including whitespace-only edits. This is intentional - nebi's job is to track "has this file changed from what was pulled?", not to interpret whether the change is meaningful. Users who want semantic comparison (e.g., "does my lock file still satisfy my manifest?") can use `pixi lock --check` independently.

#### Strategy 2: Pixi Satisfiability Check (Complementary, Future)

`pixi lock --check` can verify whether `pixi.lock` still satisfies `pixi.toml`. This detects a different kind of drift:
- Nebi digest comparison → "has the file content changed at all?"
- `pixi lock --check` → "is the lock file still valid for the manifest?"

These are complementary. For MVP, content hash is sufficient. We could layer pixi-native checks later for richer status (e.g., distinguishing "modified but lock is still valid" from "modified and lock is stale").

#### Strategy 3: Timestamp Comparison (Not Recommended)

Compare file modification times against `pulled_at` timestamp.

**Pros**: Simple, no hashing needed
**Cons**: Unreliable (files can be touched without changes, or changes can have old timestamps)

### Recommended Approach: OCI Layer Digests + Clear Semantics

1. **Store per-file layer digests at pull time** in `.nebi` metadata file and local index (these come directly from the OCI manifest, no extra computation needed)
2. **Show drift status** in `workspace list --local`
3. **Warn on activation** if workspace has drifted
4. **Distinguish "origin" from "current state"** in terminology

### Updated CLI Behavior

```bash
# List shows drift status
$ nebi workspace list --local
LOCAL WORKSPACES
  WORKSPACE           TAG       STATUS      LOCATION
  data-science        v1.0      modified    ~/project-a
  data-science        v1.0      clean       ~/project-b
  data-science        v2.0      clean       ~/.local/share/nebi/... (global)

# Status shows details about drift
$ cd ~/project-a && nebi status -v
Workspace: data-science:v1.0
Registry:  ds-team
Pulled:    2024-01-20 10:30:00
Digest:    sha256:abc123def456...

Status:    modified
  pixi.toml:  modified
  pixi.lock:  modified

Next steps:
  nebi diff              # See what changed
  nebi pull --force      # Discard local changes
  nebi push :v1.1        # Publish as new version

# Shell warns about drift but proceeds
$ nebi shell data-science:v1.0
⚠ Local copy at ~/project-a has been modified since pull.
  Origin: data-science:v1.0 (sha256:abc123...)
  Current: sha256:def456...

Activating modified local copy...
(data-science) $

# Diff command to see full changes
$ nebi diff
--- pulled (data-science:v1.0, sha256:abc123...)
+++ local
@@ pixi.toml @@
 [dependencies]
 python = ">=3.11"
+numpy = ">=1.24"
+pandas = ">=2.0"
```

### The `.nebi` Metadata File

Each pulled workspace gets a `.nebi` metadata file storing origin information and the OCI layer digests for drift detection:

```yaml
# ~/project-a/.nebi
origin:
  workspace: data-science
  tag: v1.0
  registry: ds-team
  registry_url: ghcr.io/myorg           # Full OCI registry URL
  manifest_digest: sha256:abc123def456...  # OCI manifest digest
  pulled_at: 2024-01-20T10:30:00Z
  source: oci                            # "oci" (direct registry) or "server" (nebi server API)
  server_url: https://nebi.example.com   # Only present if source: server
  server_version_id: 42                  # Server-side version ID (if source: server)

layers:
  pixi.toml:
    digest: sha256:111...   # OCI layer digest (= sha256 of file content)
    size: 2345
    media_type: application/vnd.pixi.toml.v1+toml
  pixi.lock:
    digest: sha256:222...   # OCI layer digest (= sha256 of file content)
    size: 45678
    media_type: application/vnd.pixi.lock.v1+yaml
```

The `layers` digests come directly from the OCI manifest pulled from the registry - no separate computation needed. Since nebi stores files as raw blobs (not tar layers), these digests are simply `sha256(file_content)`, making local comparison trivial.

**Source field**: Indicates how the workspace was pulled:
- `oci`: Pulled directly from an OCI registry. Use `registry_url` + `manifest_digest` to fetch content.
- `server`: Pulled via the Nebi server API. Use `server_url` + `server_version_id` to fetch content.
  The server stores versions independently of tag mutability, so fetching by version ID is always reliable.

Both source types store the `manifest_digest` for drift detection, but the fetch path
for `nebi diff` differs based on source type.

This allows:
- Drift detection without network access (just re-hash local files and compare)
- Understanding where a workspace came from
- Deciding whether local changes should be pushed as a new version

### Implications for Push

When pushing a modified workspace:

```bash
$ cd ~/project-a && nebi push data-science:v1.1
Pushing data-science:v1.1 to ds-team...

Note: This workspace was originally pulled from data-science:v1.0
  - 2 packages added to pixi.toml
  - pixi.lock updated

✓ Pushed data-science:v1.1

# The .nebi file is updated to reflect new origin
```

If user tries to push back to the same tag:

```bash
$ cd ~/project-a && nebi push data-science:v1.0
⚠ data-science:v1.0 already exists in registry with different content.
  Registry digest: sha256:abc123...
  Local digest:    sha256:def456...

Options:
  1. Push as new tag (e.g., v1.0.1 or v1.1)
  2. Overwrite existing tag (--force, may break other users)
  3. Cancel

Choice [1/2/3]: _
```

### Local Index Format

The local index is a JSON file at `~/.local/share/nebi/index.json`. It's human-readable and easy to inspect/debug during development.

```jsonc
// ~/.local/share/nebi/index.json
{
  "version": 1,
  "workspaces": [
    {
      "workspace": "data-science",
      "tag": "v1.0",
      "registry": "ds-team",
      "registry_url": "ghcr.io/myorg",
      "source": "oci",
      "path": "/home/user/project-a",
      "is_global": false,
      "pulled_at": "2024-01-20T10:30:00Z",
      "manifest_digest": "sha256:abc123def456...",
      "layers": {
        "pixi.toml": "sha256:111...",
        "pixi.lock": "sha256:222..."
      }
    },
    {
      "workspace": "data-science",
      "tag": "v1.0",
      "registry": "ds-team",
      "path": "/home/user/project-b",
      "is_global": false,
      "pulled_at": "2024-01-21T14:00:00Z",
      "manifest_digest": "sha256:abc123def456...",
      "layers": {
        "pixi.toml": "sha256:111...",
        "pixi.lock": "sha256:222..."
      }
    },
    {
      "workspace": "data-science",
      "tag": "v2.0",
      "registry": "ds-team",
      "path": "/home/user/.local/share/nebi/workspaces/data-science/v2.0",
      "is_global": true,
      "pulled_at": "2024-01-22T09:00:00Z",
      "manifest_digest": "sha256:def789...",
      "layers": {
        "pixi.toml": "sha256:333...",
        "pixi.lock": "sha256:444..."
      }
    }
  ]
}
```

**Design notes**:
- `version` field allows future schema migrations
- `path` is the unique key (one entry per directory)
- `layers` stores the OCI layer digests for drift detection (re-hash local files and compare)
- The file is read-modify-written on pull; concurrent writes are unlikely (single-user CLI) but a `.lock` file can be added later if needed
- Can be migrated to SQLite later if the index grows large enough to matter

## Global Storage Structure

Global pulls use the workspace's server-assigned UUID as the directory name, with tags as subdirectories. This keeps paths stable regardless of any future renaming or aliasing:

```
~/.local/share/nebi/
├── index.json
└── workspaces/
    └── 550e8400-e29b-41d4-a716-446655440000/
        ├── v1.0/
        │   ├── pixi.toml
        │   ├── pixi.lock
        │   └── .nebi
        └── v2.0-beta/
            ├── pixi.toml
            ├── pixi.lock
            └── .nebi
```

### Why UUIDs Instead of Names?

Using the workspace name as the directory name (e.g., `workspaces/data-science/v1.0/`) would make renaming difficult — you'd have to move files on disk, which can break running processes, installed shebangs, etc. UUIDs are stable and opaque; the index handles all human-friendly naming.

### Global Aliases

The `index.json` maps user-friendly aliases to UUID + tag locations:

```json
{
  "aliases": {
    "ds-stable": {"uuid": "550e8400-e29b-41d4-a716-446655440000", "tag": "v1.0"},
    "ds-latest": {"uuid": "550e8400-e29b-41d4-a716-446655440000", "tag": "v2.0-beta"}
  }
}
```

Usage:

```bash
nebi shell --name ds-stable    # activates the v1.0 environment
nebi shell --name ds-latest    # activates the v2.0-beta environment
```

### No Duplicate Global Copies of the Same `workspace:tag`

A given `workspace:tag` can only exist once in global storage. If a user wants a second copy of the same `workspace:tag` to experiment with independently, they should use a local (non-global) pull instead:

```bash
# One canonical global copy
nebi pull --global data-science:v1.0 --name ds-stable

# Second global pull of the same workspace:tag is blocked
nebi pull --global data-science:v1.0 --name ds-experiment
# Error: data-science:v1.0 already exists globally.
#   Location: ~/.local/share/nebi/workspaces/550e8400.../v1.0/
#   Alias: ds-stable
# Use --force to re-pull and overwrite, or pull locally to experiment.

# Want a separate copy to modify? Pull locally instead
cd ~/experiments && nebi pull data-science:v1.0
# Now you can evolve this copy independently without affecting the global one
```

This keeps global storage simple (one copy per workspace:tag, multiple aliases allowed) while local pulls provide the flexibility for experimentation and independent evolution.

## Future Considerations

1. **Cache cleanup**: `nebi cache prune` to remove orphaned content
2. **Workspace sync**: `nebi workspace sync` to update all local copies of a workspace

## Summary

### Pull Behavior

| Scenario | Behavior |
|----------|----------|
| Directory pull (new location) | Always succeeds, adds to index |
| Directory pull (same location) | Overwrites, updates index entry |
| Directory pull (different tag, same location) | Prompts to confirm overwrite |
| Global pull (new workspace:tag) | Succeeds, adds to index |
| Global pull (existing workspace:tag) | Blocked, suggests `--force` |
| Global pull (different tag) | Succeeds (separate directory) |

### Drift Detection

| Scenario | Behavior |
|----------|----------|
| `nebi workspace list --local` | Shows status: clean/modified/missing for each |
| `nebi status` / `nebi status -v` | Shows which files modified, next-step suggestions |
| `nebi diff` | Shows full unified diff of what changed |
| `nebi shell <workspace>` (modified) | Warns about drift, proceeds with local copy |
| `nebi push` (modified from origin) | Notes the origin, suggests new tag |
| `nebi push` (same tag, different content) | Blocks unless `--force` |

### Shell Activation

| Scenario | Behavior |
|----------|----------|
| `nebi shell` (in workspace dir) | Uses local copy, warns if modified |
| `nebi shell <workspace>:<tag>` | Prefers global → most recent local → prompts if multiple |
| `nebi shell <workspace>:<tag>` (modified) | Warns about drift, activates anyway |

### Key Concepts

- **Origin**: The `workspace:tag` from which a local copy was pulled
- **Drift**: When local files differ from what was originally pulled
- **Status**: `clean` (matches origin), `modified` (changed locally), `missing` (path gone)
