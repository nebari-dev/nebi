# Issue: Failed Push Ghost Workspaces & Pull Should Skip When Already Up to Date

Two related edge cases in the push/pull workflows.

## Problem 1: Failed Push Shows Workspace in List

If a push fails after the workspace is created on the server (e.g., network error during OCI push, auth failure to registry), the workspace still shows up in `nebi workspace list`. The user sees a workspace that was never successfully published.

```bash
$ nebi push new-workspace:v1.0
Creating workspace "new-workspace"...
Created workspace "new-workspace"
Waiting for environment to be ready... done
Pushing new-workspace:v1.0 to my-registry...
Error: Failed to push new-workspace:v1.0: connection refused

$ nebi workspace list
NAME             STATUS   PACKAGE MANAGER   OWNER
new-workspace    ready    pixi              alice    ← ghost workspace, no tags
other-workspace  ready    pixi              alice
```

### Root Cause

The push command (`cmd/nebi/push.go`) has two independent phases:

1. **Phase A: Workspace Creation** — `CreateEnvironment()` persists an `Environment` record to the server DB, queues a creation job, and waits for status `ready`. This is permanent and immediate.

2. **Phase B: OCI Publish** — `PublishEnvironment()` pushes layers to the OCI registry and (only on success) creates a `Publication` record.

If Phase A succeeds but Phase B fails, the workspace exists with no publications. The server-side handler (`internal/api/handlers/environment.go` `CreateEnvironment`) has no rollback mechanism.

The server-side `PublishEnvironment` handler is already "transactional" for the publish step — it only creates the `Publication` DB record after `oci.PublishEnvironment()` succeeds. So the problem is specifically the gap between workspace creation and first publication.

### Partial Success Sub-Case

There's a subtle sub-case within a single `PublishEnvironment` call: What if the OCI push succeeds but the DB write (`h.db.Create(&publication)`) fails? Then:
- The OCI registry has the content (layers + manifest + tag)
- The server DB has no publication record
- The content is orphaned in the registry

This is low-probability but worth noting. The user can retry `nebi push` and it will overwrite the tag. A DB transaction wrapping both operations would prevent this.

### Proposed Solution

**Approach: Better UX on failure + filtering (not rollback)**

Rollback (deleting the workspace on push failure) is too destructive — the workspace creation succeeded legitimately, and the user likely wants to retry the push. Instead:

1. **CLI: Suggest cleanup on failure** — When `PublishEnvironment` fails and the workspace was just created in the same command invocation, show:
   ```
   Error: Failed to push new-workspace:v1.0: connection refused

   Note: Workspace "new-workspace" was created but has no published tags.
     To retry:  nebi push new-workspace:v1.0
     To remove: nebi workspace delete new-workspace
   ```

2. **Server: Add publication filter to ListEnvironments** — Add an optional query parameter `?has_publications=true` that excludes workspaces with zero publications. The CLI could use this for display or offer a `--published` flag on `nebi workspace list`.

3. **CLI: Show tag count in workspace list** — Add a `TAGS` column showing publication count:
   ```
   NAME             STATUS   TAGS   PACKAGE MANAGER   OWNER
   new-workspace    ready    0      pixi              alice
   other-workspace  ready    3      pixi              alice
   ```
   This makes the ghost workspace immediately obvious without hiding it.

**Why not rollback:**
- The workspace creation is a valid operation independent of publishing
- Transient failures (network blip) shouldn't destroy server state
- The user explicitly named the workspace — they probably want to keep it
- Matches git mental model: `git init` + failed `git push` doesn't delete the repo

## Problem 2: Pull Should Skip Overwrite When Content Already Matches

When running `nebi pull workspace:tag` on a directory that already contains the exact same content (same `workspace:tag`, local files unmodified), the pull unnecessarily re-fetches and overwrites files. It should detect "already up to date" and short-circuit.

```bash
$ nebi pull data-science:v1.0
Pulled data-science:v1.0 (version 3) → ~/project

# Run again immediately — should be a no-op
$ nebi pull data-science:v1.0
Pulled data-science:v1.0 (version 3) → ~/project    ← wasteful re-fetch

# Desired behavior:
$ nebi pull data-science:v1.0
Already up to date (data-science:v1.0)
```

### Root Cause

In `cmd/nebi/pull.go`, the `handleDirectoryPull()` function has a check for same `workspace:tag` at the same path that simply returns the output directory without any content comparison:

```go
if existing != nil && existing.Workspace == workspace && existing.Tag == tag {
    // Same workspace:tag to same directory - re-pull (overwrite), no prompt needed
    return outputDir, nil
}
```

After this, the pull always proceeds to fetch `pixi.toml` and `pixi.lock` from the server and overwrite local files.

### When to Skip

The pull can be skipped when ALL of these conditions are true:

1. `.nebi` file exists in the target directory
2. The stored `workspace` and `tag` match what's being pulled
3. The stored `manifestDigest` matches the publication's current digest (meaning the tag hasn't been moved/overwritten remotely)
4. Local files are `clean` (drift detection passes — local file sha256 matches stored layer digests)

If condition 3 fails (tag has moved), the remote has newer content and we should proceed with the pull.
If condition 4 fails (local files modified), there are sub-cases discussed below.

### Proposed Solution

**Add a `checkAlreadyUpToDate()` function** in `pull.go`, called after version resolution but before fetching file content:

```go
// After resolving versionNumber and manifestDigest, before fetching files:
if !pullForce {
    if upToDate, msg := checkAlreadyUpToDate(outputDir, workspaceName, tag, manifestDigest); upToDate {
        fmt.Println(msg)
        return
    }
}
```

The `checkAlreadyUpToDate` function logic:

```
func checkAlreadyUpToDate(dir, workspace, tag, remoteDigest string) (bool, string) {
    1. if !nebifile.Exists(dir) → return false (no metadata, can't compare)
    2. nf, err := nebifile.Read(dir) → return false on error
    3. if nf.Origin.Workspace != workspace || nf.Origin.Tag != tag → return false
    4. if remoteDigest != "" && nf.Origin.ManifestDigest != remoteDigest → return false (tag moved)
    5. Run drift.Check(dir) to compare local file hashes against stored layer digests
    6. if clean → return true, "Already up to date (workspace:tag)"
    7. if modified:
       - Warn: "Local files have been modified since last pull."
       - If !pullYes: prompt "Re-pull to discard local changes? [y/N]"
       - If user says no → return true, "Pull skipped (local modifications preserved)"
       - If user says yes or --yes → return false (proceed with pull)
    8. return false (default: proceed with pull)
}
```

### Edge Cases

| Scenario | Behavior |
|----------|----------|
| Same workspace:tag, clean, same digest | Skip: "Already up to date" |
| Same workspace:tag, clean, different digest | Pull: tag was updated remotely |
| Same workspace:tag, modified, same digest | Prompt: "Local modifications exist. Re-pull?" |
| Same workspace:tag, modified, different digest | Prompt: "Local modifications AND remote updates. Re-pull?" |
| Different workspace:tag | Existing behavior: prompt for overwrite |
| No .nebi file exists | Existing behavior: proceed with pull |
| `--force` flag | Always proceed, skip all checks |

### Relationship to Drift Detection Design

This leverages the existing infrastructure from `docs/design/duplicate-pulls.md`:
- `.nebi` layer digests for offline content comparison (`nebifile.ComputeDigest` + `drift.Check`)
- `manifestDigest` for detecting remote tag mutation
- The status values (`clean`/`modified`/`missing`) already defined

The `--force` flag provides an escape hatch to always re-pull regardless of state.

## Implementation Priority

1. **Pull skip** (Problem 2) — Higher priority. Prevents unnecessary network calls and file writes. Straightforward to implement using existing drift detection.

2. **Push failure UX** (Problem 1) — Lower priority. The current behavior is not broken per se (the workspace exists, the user can retry or delete it). The fix is primarily informational messaging.

## Files Involved

| File | Changes Needed |
|------|---------------|
| `cmd/nebi/pull.go` | Add `checkAlreadyUpToDate()` function, call before fetch |
| `cmd/nebi/push.go` | Track `justCreated` bool, show cleanup suggestion on failure |
| `internal/api/handlers/environment.go` | (Optional) Add `has_publications` filter to `ListEnvironments` |
| `cmd/nebi/workspace.go` | (Optional) Show tag count column in `runWorkspaceList` |
