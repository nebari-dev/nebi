# Design: Workspace Diff and Status Workflows

> **Relationship to other docs**: This document builds on the drift detection and `.nebi` metadata
> design already specified in [`duplicate-pulls.md`](./duplicate-pulls.md) (see "Local Environment
> Drift" section). That document defines **what** is tracked (OCI layer digests, `.nebi` YAML format,
> status values). This document defines **how users interact** with that information: the CLI commands,
> output formats, and remote comparison workflows.

## Problem Statement

Users who pull Pixi workspaces from OCI registries need visibility into differences between:
1. Their local workspace and what they originally pulled (drift detection)
2. Their local workspace and what's currently in the registry (remote changes)
3. Two different versions/tags in the registry (upgrade planning)

This is especially important in multi-user environments where:
- Users modify local workspaces after pulling (add packages, tweak configs)
- Others may update the same workspace:tag in the registry (mutable tags like `latest`)
- Users need to decide whether to push local changes or pull remote updates

## Prerequisites

This design assumes the following from `duplicate-pulls.md` are implemented:
- `.nebi` metadata file (YAML) written on pull with `origin:` and `layers:` sections
- OCI layer digests stored per-file for byte-level drift detection
- Local index (`~/.local/share/nebi/index.json`) tracking all pulled workspaces

## User Stories and Workflows

### Workflow 1: "Have I made local changes?"

**Persona**: Data scientist who pulled `data-science:v1.0` a week ago, made some tweaks, and forgot what they changed.

**Need**: See local changes vs what was originally pulled.

```bash
$ cd ~/my-project
$ nebi status

Workspace: data-science:v1.0 (from ds-team registry)
Pulled: 2025-01-15 (7 days ago)
Digest: sha256:abc123...

Status:    modified
  pixi.toml:  modified
  pixi.lock:  modified

Next steps:
  nebi diff              # See what changed
  nebi pull --force      # Discard local changes
  nebi push :v1.1        # Publish as new version
```

### Workflow 2: "Is the registry version newer?"

**Persona**: User with local copy, wants to know if someone pushed updates to the registry.

**Need**: Compare their local version vs current registry version.

```bash
$ cd ~/my-project
$ nebi status --remote

Workspace: data-science:v1.0 (from ds-team registry)
Pulled:        2025-01-15 (7 days ago)
Origin digest: sha256:abc123...

Local status: modified (pixi.toml, pixi.lock)

Remote status:
  ⚠ Tag 'v1.0' now points to sha256:xyz789... (was sha256:abc123... when pulled)
  The tag has been updated since you pulled.

Run 'nebi diff' to see your local changes (vs what you pulled).
Run 'nebi diff --remote' to see diff against current remote.
Run 'nebi pull --force' to update local copy (overwrites local changes).
```

Note: This clearly distinguishes "what did I change?" from "what changed remotely?"
by always reporting both the origin digest and the current tag digest. See "The Mutable
Tag Problem" section below for the full rationale.

### Workflow 3: "What changed between tags?"

**Persona**: User is on `v1.0`, considering upgrading to `v2.0`.

**Need**: Diff between two registry versions.

```bash
$ nebi diff data-science:v1.0 data-science:v2.0

--- data-science:v1.0
+++ data-science:v2.0
@@ pixi.toml @@
 [dependencies]
-python = ">=3.11"
+python = ">=3.12"
-tensorflow = ">=2.14.0"
+torch = ">=2.0.0,<3"
+transformers = ">=4.35.0"

@@ pixi.lock (summary) @@
 45 packages added, 12 removed, 8 updated
```

### Workflow 4: "Can I push my changes?"

**Persona**: User modified local workspace, wants to push as new tag.

**Need**: Preview what would be pushed, detect potential conflicts.

```bash
$ cd ~/my-project
$ nebi push data-science:v1.1 --dry-run

Would push data-science:v1.1 to ds-team registry

--- origin (data-science:v1.0)
+++ local (to be pushed)
@@ pixi.toml @@
 [dependencies]
-numpy = ">=2.0.0,<3"
+numpy = ">=2.4.1,<3"
+scipy = ">=1.17.0,<2"

@@ pixi.lock (summary) @@
 12 packages changed

Run without --dry-run to push.
```

### Workflow 5: "Merge remote changes" (Future Enhancement)

**Persona**: Local has changes, remote has changes, user wants both.

**Complexity**: This requires conflict resolution, similar to git merge.

**MVP Approach**: Out of scope for initial implementation. Recommend:
```bash
$ nebi status --remote
⚠ Both local and remote have changes!

Options:
  1. nebi pull --force     # Discard local, use remote
  2. nebi push :v1.1       # Push local as new tag
  3. Manual merge: review 'nebi diff --remote' and edit files
```

## The Mutable Tag Problem

A critical design consideration: **tags can be overwritten** in OCI registries. This creates
ambiguity about what "remote" means.

### Git Analogy

This is the exact same problem git solves. The mapping is direct:

| Git Concept | Nebi Equivalent | Description |
|-------------|----------------|-------------|
| Working tree | Local `pixi.toml`/`pixi.lock` | Files on disk, may be modified |
| `HEAD` / last commit | Origin digest in `.nebi` | Immutable snapshot you started from |
| `origin/main` | Current tag in registry | What the mutable ref points to *now* (stale until fetched) |
| Commit hash (`abc123`) | Content digest (`sha256:...`) | Immutable content identifier |
| Branch name (`main`) | Tag name (`v1.0`) | Mutable pointer that can move |
| `git fetch` | `nebi status --remote` | Update knowledge of remote state |
| `git diff HEAD` | `nebi diff` | Local changes vs your starting point |
| `git diff origin/main` | `nebi diff --remote` | Local vs what remote has *now* |
| `git diff ref1..ref2` | `nebi diff ref1 ref2` | Between two versions |
| `git status` | `nebi status` | Quick "is anything dirty?" |

The key git principle that applies: **`origin/main` is a local cache of remote state that only
updates on explicit `git fetch`**. Git never auto-syncs. Similarly, nebi's knowledge of the
current tag state is stale until you explicitly check with `--remote`.

And just like in git, the immutable unit (commit hash / content digest) is always safe to
reference, while the mutable reference (branch / tag) can move independently.

### The Three States

When a user has a local workspace, there are potentially three distinct states (analogous to
git's working tree / HEAD / origin/branch):

```
                ┌─────────────────────────────────────────────────────────────┐
                │                         Registry                            │
                │                                                             │
  ┌─────────┐  │  ┌─────────────┐              ┌──────────────────────┐      │
  │  LOCAL   │  │  │  ORIGIN     │              │  CURRENT TAG         │      │
  │          │  │  │  (digest)   │              │  (tag resolution)    │      │
  │ Modified │  │  │ sha256:aaa  │              │  sha256:bbb          │      │
  │ pixi.toml│  │  │ (immutable) │              │  (may have moved!)   │      │
  └─────────┘  │  └─────────────┘              └──────────────────────┘      │
                │                                                             │
                └─────────────────────────────────────────────────────────────┘

Timeline:
  Day 1: User pulls data-science:v1.0 → gets sha256:aaa
  Day 3: Someone pushes new content to data-science:v1.0 → now sha256:bbb
  Day 4: User modifies local pixi.toml
  Day 5: User runs nebi status --remote
```

Now there are three versions:
1. **Origin** (`sha256:aaa`): What was originally pulled. Stored in `.nebi`. Immutable.
2. **Local**: User's current files on disk. May differ from origin.
3. **Current tag** (`sha256:bbb`): What `data-science:v1.0` resolves to *now*. May differ from origin if tag was overwritten.

### What Each Command Compares

| Command | Source | Target | Uses |
|---------|--------|--------|------|
| `nebi diff` | Origin (by digest) | Local files | Always correct — uses immutable digest |
| `nebi diff --remote` | Current tag (by tag) | Local files | Shows what's different from *current* registry state |
| `nebi status` | Origin layer digests | Local file hashes | Offline, no ambiguity |
| `nebi status --remote` | Origin manifest digest | Current tag manifest digest | Detects tag mutation |

### Key Insight: `nebi diff` (no args) Is Always Reliable

Because `nebi diff` uses the **digest** stored in `.nebi` (not the tag), it always fetches
the exact content the user originally pulled, regardless of whether the tag has since moved.
The OCI spec guarantees that content referenced by digest is immutable.

```bash
# This always works correctly - fetches by digest, not tag
$ nebi diff
Comparing sha256:aaa (originally pulled as data-science:v1.0) → local
```

### `nebi status --remote` Must Detect Tag Mutation

When checking remote, we resolve the tag and compare **both** against origin and local:

```bash
$ nebi status --remote

Workspace: data-science:v1.0 (from ds-team registry)
Pulled:    2025-01-15 (digest sha256:aaa...)

Local status:  modified (pixi.toml changed)

Remote status:
  ⚠ Tag 'v1.0' now points to sha256:bbb... (was sha256:aaa... when pulled)
  The tag has been updated since you pulled.

Recommendations:
  • To see your local changes:     nebi diff
  • To see what the tag changed to: nebi diff --remote
  • To discard local & get latest:  nebi pull --force
  • To push your version as new:    nebi push data-science:v1.1
```

### `nebi diff --remote` Shows Current Tag vs Local

This deliberately compares against what the tag points to *now*, because the user is asking
"how does my workspace compare to what's currently published?":

```bash
$ nebi diff --remote

Note: Tag 'v1.0' has been updated since you pulled (was sha256:aaa...).
      Showing diff against current remote content.

--- remote (data-science:v1.0, current: sha256:bbb...)
+++ local
@@ pixi.toml @@
 [dependencies]
+scipy = ">=1.17.0,<2"
-pandas = ">=2.0.0,<3"
 ...
```

### Immutable Tags Simplify Everything

If Nebi enforces immutable tags (you can't overwrite `v1.0` once pushed), then:
- Origin digest always equals current tag digest
- `nebi diff` and `nebi diff --remote` give the same result (minus local changes)
- `nebi status --remote` can only report "no remote changes" (the tag can't move)
- The only useful remote check is "does a *newer* tag exist?"

This is worth considering as a product decision. Immutable tags:
- **Pros**: Simpler mental model, reproducibility, no accidental overwrites, fewer edge cases
- **Cons**: Less flexibility (can't fix a bad push to same tag), `latest` pattern doesn't work

**If immutable tags are adopted**, `nebi status --remote` could instead check:
```bash
$ nebi status --remote
Workspace: data-science:v1.0 (from ds-team registry)
Status:    modified locally

Newer tags available:
  v1.1 (pushed 2025-01-18)
  v2.0 (pushed 2025-01-20)

Run 'nebi diff data-science:v1.0 data-science:v2.0' to compare.
```

### Why Not `nebi fetch`?

We considered a separate `nebi fetch` command (like `git fetch`) that updates local knowledge
of remote state, followed by `nebi status` or `nebi diff` to display it. We chose `--remote`
flags instead for these reasons:

1. **No persistent local ref to update.** In git, `fetch` updates `origin/main` — a real file
   in `.git/refs/` that persists between commands. Nebi has no equivalent. We'd have to invent
   a local cache of "last known remote digest" and manage its lifecycle, staleness, etc.

2. **Fewer commands to learn.** Users want one answer: "am I up to date?" Having to run
   `nebi fetch && nebi status` is two steps where one suffices.

3. **Nebi's payloads are tiny.** Git fetch downloads potentially large objects. Nebi's
   "fetch" is a single HTTP request for a manifest (~1KB). There's no performance reason
   to separate the fetch from the display.

4. **No offline replay needed.** In git, you fetch once and then run many commands against
   the cached refs (diff, log, merge-base, etc.). In Nebi, `--remote` is typically a
   one-shot question: "has the tag moved?" There's no sequence of offline operations that
   benefit from cached remote state.

5. **`nebi status` (no `--remote`) already works offline.** The offline use case is covered:
   `nebi status` compares local hashes against `.nebi` layer digests with zero network.
   The `--remote` flag is an explicit opt-in to "yes, hit the network now."

If we later find users running `nebi status --remote` and `nebi diff --remote` back-to-back
(redundant network calls), we can add a short-lived cache (TTL ~60s) transparently. But
that's an implementation optimization, not a UX-level `fetch` command.

### Recommendation

**For MVP**: Support both mutable and immutable tags at the protocol level, but:
1. Always store and use the **manifest digest** in `.nebi` for origin comparison
2. `nebi diff` (no args) always fetches by digest — safe regardless of tag mutability
3. `nebi diff --remote` resolves the tag — shows current state
4. `nebi status --remote` compares digests — clearly reports if tag has moved
5. If tags are immutable, `--remote` additionally shows newer available tags

**Defer the policy decision** on whether to enforce immutable tags. The diff/status
commands work correctly either way because they track digests, not just tags.

## Command Naming Decision

`duplicate-pulls.md` uses `nebi workspace diff` and `nebi workspace info` as subcommands.
This document proposes **top-level** `nebi status` and `nebi diff` instead, for the following reasons:

1. **Frequency of use**: Status/diff are high-frequency operations (like `git status`/`git diff`). Extra typing hurts UX.
2. **Precedent**: git, terraform, helm all use top-level status/diff commands.
3. **Distinction from `workspace list`**: `nebi workspace list --local` shows *all* workspaces. `nebi status` shows *this* workspace's state.

**Proposed resolution**: Both forms work as aliases:
- `nebi status` = `nebi workspace info` (with enhanced output)
- `nebi diff` = `nebi workspace diff` (with enhanced output)

The `workspace` subcommand form would be documented but the short form preferred in examples.

## Proposed CLI Commands

### `nebi status`

Show the current state of a local workspace. This extends the `nebi workspace info` concept
from `duplicate-pulls.md` with remote checking.

```
nebi status [--remote] [--json] [-v|--verbose] [-C|--path PATH]

Flags:
  --remote    Also check remote registry for updates
  --json      Output as JSON for scripting
  -v          Verbose output (full digests, next-step suggestions)
  -C, --path  Operate on workspace at specified path (like git -C)
```

**Behavior**:
1. Read `.nebi` metadata file in current directory
2. Re-hash local `pixi.toml` and `pixi.lock`, compare against stored layer digests
3. Report status: `clean` or `modified` (per the status values defined in `duplicate-pulls.md`)
4. If `--remote`: fetch remote manifest digest and compare against local `manifest_digest`

**Output (compact, default)**:
```
$ nebi status
data-science:v1.0 (ds-team)  •  pulled 7 days ago  •  modified
```

**Output (verbose)**:
```
$ nebi status -v
Workspace: data-science:v1.0
Registry:  ds-team
Pulled:    2025-01-15 10:30:00 (7 days ago)
Digest:    sha256:abc123def456...

Status:    modified
  pixi.toml:  modified
  pixi.lock:  modified

Next steps:
  nebi diff              # See what changed
  nebi pull --force      # Discard local changes
  nebi push :v1.1        # Publish as new version
```

**Output (JSON)**:
```json
{
  "workspace": "data-science",
  "tag": "v1.0",
  "registry": "ds-team",
  "pulled_at": "2025-01-15T10:30:00Z",
  "origin_digest": "sha256:abc123...",
  "local": {
    "pixi_toml": "modified",
    "pixi_lock": "modified"
  },
  "remote": {
    "current_tag_digest": "sha256:xyz789...",
    "tag_has_moved": true,
    "origin_still_exists": true
  }
}
```

Note: `origin_digest` is what the user pulled (immutable). `current_tag_digest` is what the
tag resolves to *now*. If `tag_has_moved` is true, the tag was overwritten after pull.
`origin_still_exists` indicates whether the original digest is still accessible in the
registry (it should always be, unless the registry was garbage-collected).

### `nebi diff`

Show detailed differences between workspace versions. While `nebi status` answers "has anything
changed?", `nebi diff` answers "what exactly changed?".

```
nebi diff [source] [target] [--remote] [--json] [--lock] [--toml] [-C|--path PATH]

Arguments:
  source   Source reference (default: pulled version from .nebi)
  target   Target reference (default: local files). Can be a path or workspace:tag.

Flags:
  --remote       Compare local against current remote (shorthand)
  --json         Output as JSON for scripting
  --lock         Show lock file diff (default: summary only)
  --toml         Show only pixi.toml diff
  -C, --path     Operate on workspace at specified path (like git -C)
```

**Usage patterns**:
```bash
# Local changes vs what was pulled (uses .nebi layer digests as baseline)
nebi diff

# Local vs current remote (fetches remote layers for comparison)
nebi diff --remote

# Between two registry versions (fetches both from registry)
nebi diff data-science:v1.0 data-science:v2.0

# Between registry version and local
nebi diff data-science:v1.0 .
```

**How `nebi diff` (no args) gets the original content**:

The `.nebi` file stores layer digests but not the original file content. Two approaches:

1. **Re-fetch from server** (simpler): Use the stored `server_version_id` to fetch the exact
   original content via the Nebi server API. Version IDs are immutable and guaranteed to match.
2. **Stash original on pull** (faster, offline): Save a copy of pulled files to
   `~/.cache/nebi/pulled/<digest>/`. Enables offline diff but costs disk space.

**Recommendation for MVP**: Re-fetch from server. For `nebi status` (just modified/clean),
no network is needed (local hash comparison). For `nebi diff` (show *what* changed), fetching
the small files (~50KB) from the server is fast enough.

**Output format (unified diff, like git)**:
```
--- pulled (data-science:v1.0, sha256:abc123...)
+++ local
@@ pixi.toml @@
 [workspace]
-version = "0.1.0"
+version = "0.2.0"

 [dependencies]
-numpy = ">=2.0.0,<3"
+numpy = ">=2.4.1,<3"
+scipy = ">=1.17.0,<2"
-old-package = "*"

@@ pixi.lock (summary) @@
 12 packages changed:
   Added (3):    scipy, liblapack, libopenblas
   Removed (1):  old-package
   Updated (8):  numpy (2.0.0 → 2.4.1), python (3.11 → 3.12), ...

[Use --lock for full lock file diff]
```

**Format**: Standard unified diff (`+`/`-`/` ` prefix), matching git conventions.
Context lines (unchanged) are shown with a space prefix for readability.

**JSON output**:
```json
{
  "source": {
    "type": "pulled",
    "workspace": "data-science",
    "tag": "v1.0",
    "digest": "sha256:abc123..."
  },
  "target": {
    "type": "local",
    "path": "/home/user/my-project"
  },
  "pixi_toml": {
    "added": [
      {"section": "dependencies", "key": "scipy", "value": ">=1.17.0,<2"}
    ],
    "removed": [
      {"section": "dependencies", "key": "tensorflow", "value": ">=2.14.0"}
    ],
    "modified": [
      {"section": "dependencies", "key": "numpy", "old": ">=2.0.0,<3", "new": ">=2.4.1,<3"}
    ]
  },
  "pixi_lock": {
    "packages_added": 3,
    "packages_removed": 1,
    "packages_updated": 8,
    "added": ["scipy 1.17.0", "liblapack 3.11.0", "libopenblas 0.3.30"],
    "removed": ["old-package 1.0.0"],
    "updated": [
      {"name": "numpy", "old": "2.0.0", "new": "2.4.1"},
      {"name": "python", "old": "3.11.0", "new": "3.12.0"}
    ]
  }
}
```

### `nebi push --dry-run`

Extension to existing `push` command. Preview what would be published.

```bash
$ nebi push data-science:v1.1 --dry-run

Would push data-science:v1.1 to ds-team registry

--- origin (data-science:v1.0)
+++ local (to be pushed)
@@ pixi.toml @@
 [dependencies]
-numpy = ">=2.0.0,<3"
+numpy = ">=2.4.1,<3"
+scipy = ">=1.17.0,<2"

@@ pixi.lock (summary) @@
 12 packages changed

Run without --dry-run to push.
```

This reuses the same diff logic as `nebi diff` but frames the output as "what you're about to publish".

## What Gets Compared

### pixi.toml (Primary - Semantic Diff)

The manifest file contains human-edited configuration:
- `[workspace]` - name, version, channels, platforms
- `[dependencies]` - direct dependencies with version specs
- `[pypi-dependencies]` - PyPI packages
- `[tasks]` - defined tasks
- `[feature.*]` - feature-specific configs

**Diff strategy**: Parse TOML, compare structured data, show semantic diff.
This means we understand that `numpy = ">=2.0"` → `numpy = ">=2.4"` is a version bump,
not just a text change.

### pixi.lock (Secondary - Package-Level Diff)

The lock file contains resolved dependency graph:
- Exact versions of all transitive dependencies
- Download URLs and checksums
- Platform-specific package lists

**Diff strategy**:
- **Summary mode (default)**: Count added/removed/updated packages, show top changes
- **Full mode (`--lock`)**: Show complete package-by-package diff

The lock file is YAML. We parse it to extract the package list and compare at the
package name + version level, rather than raw line diff.

### OCI Manifest Digest (Quick Check)

The manifest digest (`sha256:...`) uniquely identifies the exact content in the registry.
Used for:
- **`nebi status`**: Quick "has anything changed?" without parsing files
- **`nebi status --remote`**: Quick "has remote been updated?" with a single API call
- **`nebi diff`**: Deciding whether to fetch and compare, or report "no changes"

## Fetching Remote Without Full Pull

To compare against remote, we need to fetch the manifest and potentially file contents
without writing them locally.

### Two Fetch Modes: By Version ID vs By Tag

This distinction is critical for correctness:

1. **By Version ID** (for `nebi diff` — compare against origin):
   - Fetches the exact content originally pulled, immutable
   - Uses `server_version_id` from `.nebi` metadata
   - `GET /api/v1/environments/:id/versions/:version/pixi-toml`
   - `GET /api/v1/environments/:id/versions/:version/pixi-lock`

2. **By Tag** (for `nebi diff --remote` — compare against current):
   - Resolves the tag to its current content, which may differ from what was pulled
   - Uses the tag name to look up the current version via server API

### Digest-Only Check (for `nebi status --remote`)

Query the server to get the current manifest digest for the tag:

```go
// Resolve the tag to its current digest via server API
currentVersion, _ := client.GetVersionByTag(ctx, workspaceID, tag)

// Compare against what we originally pulled
originDigest := nebiFile.Origin.ManifestDigest
currentDigest := currentVersion.ManifestDigest

if originDigest != currentDigest {
    // Tag has been moved/overwritten since pull!
    fmt.Println("⚠ Tag has been updated since you pulled")
}
```

This is a single API call to the Nebi server.

### Content Fetch (for `nebi diff` and `nebi diff --remote`)

To show *what* changed, fetch the file contents from the server:

```go
// For "nebi diff" (vs origin): fetch by version ID (immutable)
pixiToml, _ := client.GetVersionPixiToml(ctx, workspaceID, nebiFile.Origin.ServerVersionID)
pixiLock, _ := client.GetVersionPixiLock(ctx, workspaceID, nebiFile.Origin.ServerVersionID)

// For "nebi diff --remote" (vs current tag): resolve tag first, then fetch
currentVersion, _ := client.GetVersionByTag(ctx, workspaceID, tag)
pixiToml, _ := client.GetVersionPixiToml(ctx, workspaceID, currentVersion.ID)
pixiLock, _ := client.GetVersionPixiLock(ctx, workspaceID, currentVersion.ID)

// Compare content in memory against local files
```

For Nebi workspaces, these are just two small files:
- `pixi.toml`: ~2-10 KB typically
- `pixi.lock`: ~50-200 KB typically

The Nebi server stores version content independently of tag mutability, so fetching by
`server_version_id` is always reliable. The `.nebi` metadata stores both the version
ID and the manifest digest — see `duplicate-pulls.md` for the full `.nebi` format.

### What If the Origin Version Is Gone?

In rare cases, a server might have purged old versions. In this case:

```bash
$ nebi diff
Error: Origin version (ID: 42) is no longer available on the server.
  The tag 'v1.0' now points to a different version.

  Options:
    nebi diff --remote    Compare against current tag content
    nebi status           Check local modification status (offline, still works)
```

`nebi status` (without `--remote`) still works because it only compares local file hashes
against the stored layer digests in `.nebi` — no network needed.

## Interaction with Existing Drift Detection

`duplicate-pulls.md` already defines:
- `workspace list --local` shows status column (`clean`/`modified`/`missing`)
- `nebi shell` warns when workspace has drifted
- `nebi push` blocks pushing to same tag with different content

This design adds the **detail layer** on top of those:

| Existing (duplicate-pulls.md) | New (this doc) |
|-------------------------------|----------------|
| Status: `modified` | `nebi status`: shows which files, quick summary |
| "Warns about drift" | `nebi diff`: shows exactly what changed |
| "Blocks push" | `nebi push --dry-run`: preview before attempting |
| "Registry-side drift" | `nebi status --remote`: detect and explain |

## Implementation Considerations

### TOML Semantic Diff

For meaningful pixi.toml diff, parse and compare structurally:

```go
// Using github.com/pelletier/go-toml/v2 (already in go.mod)
var oldToml, newToml map[string]interface{}
toml.Unmarshal(oldContent, &oldToml)
toml.Unmarshal(newContent, &newToml)

// Walk the tree, compare sections
diffs := compareTomlMaps(oldToml, newToml, "")
```

Key sections to handle:
- `[dependencies]`: show as package add/remove/version-change
- `[workspace]`: show field-by-field changes
- `[tasks]`: show as task add/remove/modify
- Other sections: generic key-value diff

### Lock File Package Diff

Parse the YAML lock file to extract package names and versions:

```go
type PixiLock struct {
    Version  int `yaml:"version"`
    Packages []struct {
        Name    string `yaml:"name"`
        Version string `yaml:"version"`
        // ... other fields not needed for diff
    } `yaml:"packages"`
}
```

Then compute set differences: added, removed, version-changed.

### Error Handling

| Scenario | Behavior |
|----------|----------|
| No `.nebi` file | Error: "Not a nebi workspace. Run 'nebi pull' first." |
| `.nebi` file corrupted | Error: "Invalid .nebi metadata. Re-pull to fix." |
| Remote unreachable | Warning: "Cannot reach registry. Showing local status only." |
| pixi.toml missing | Error: "pixi.toml not found. Was it deleted?" |
| pixi.lock missing | Warning: "pixi.lock not found (lock diff unavailable)." |
| Ref not found in registry | Error: "Tag 'v2.0' not found in registry 'ds-team'." |

### Exit Codes

For scripting (`nebi status`, `nebi diff`):
- `0`: No differences / clean
- `1`: Differences detected
- `2`: Error (invalid args, network error, parse failure)

This matches `git diff` and `terraform plan -detailed-exitcode` conventions.

## MVP vs Future Enhancements

### MVP (Phase 1)

| Feature | Notes |
|---------|-------|
| `nebi status` | Local drift check using .nebi layer digests (offline) |
| `nebi status --remote` | Fetch remote manifest digest, compare |
| `nebi diff` | Fetch original content, show pixi.toml semantic diff |
| `nebi diff --json` | JSON output for tooling/CI |
| `nebi push --dry-run` | Preview push using same diff logic |

### Phase 2

| Feature | Notes |
|---------|-------|
| `nebi diff <ref1> <ref2>` | Compare two registry versions |
| `nebi diff --lock` | Detailed lock file package diff |
| Lock file package-level diff | Parse YAML, compare package sets |
| Manifest caching | `~/.cache/nebi/` with TTL |

### Phase 3 (Future)

| Feature | Notes |
|---------|-------|
| `nebi merge` | Interactive merge of local + remote changes |
| `nebi stash` | Save local changes before pull --force |
| Three-way diff | Show local changes, remote changes, and common ancestor |
| Integration with `nebi update` | Auto-diff before update |

## Comparison with Similar Tools

| Feature | Nebi (Proposed) | Docker | Helm Diff | Terraform |
|---------|-----------------|--------|-----------|-----------|
| Local vs original | `nebi status` | `docker diff` (A/C/D) | N/A | `terraform plan` |
| Local vs remote | `nebi status --remote` | N/A | `helm diff upgrade` | `terraform plan` |
| Between versions | `nebi diff v1 v2` | N/A | `helm diff revision` | N/A |
| Output format | Semantic + JSON | File-level markers | Unified diff | Custom + JSON |
| Preview push | `nebi push --dry-run` | N/A | `helm diff upgrade` | `terraform plan` |
| Offline support | `status` works offline | `diff` works offline | No | No |

**Design influences**:
- **Terraform**: Semantic diff output format (+ / - / ~ symbols), JSON machine output, exit codes
- **Helm diff**: Preview what an upgrade would change before doing it
- **Docker diff**: Simple A/C/D status for quick checks
- **Git**: Separate `status` (quick) vs `diff` (detailed) commands

## Summary

| Command | Purpose | When | MVP? |
|---------|---------|------|------|
| `nebi status` | Quick drift check | Before starting work | Yes |
| `nebi status --remote` | Check for remote updates | Before pull/push decisions | Yes |
| `nebi diff` | Detailed local changes | When you need to see what changed | Yes |
| `nebi diff --remote` | Local vs current remote | Before deciding to pull | Yes |
| `nebi diff ref1 ref2` | Compare registry versions | Planning upgrades | Phase 2 |
| `nebi push --dry-run` | Preview what would be pushed | Before publishing | Yes |

The design prioritizes:
1. **Digest-first**: All origin comparisons use immutable digests, not tags — correct regardless of tag mutability
2. **Offline first**: `nebi status` works without network (hash comparison)
3. **Progressive detail**: status → diff → diff --lock (increasing verbosity)
4. **Familiarity**: Output inspired by git, terraform, helm
5. **Scriptability**: JSON output and meaningful exit codes
6. **Efficiency**: Manifest-only fetch when possible, full fetch only for content diff
7. **Consistency**: Builds on `.nebi` and drift detection from `duplicate-pulls.md`
8. **Policy-agnostic**: Works correctly whether tags are mutable or immutable
