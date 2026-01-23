# Issue: Post-Push `.nebi` and `index.json` Not Updated

## Problem

When a user pulls `workspace:tag` (e.g., `data-science:v1`), modifies it locally, and pushes to a **new tag** (e.g., `data-science:v2`), running `nebi workspace list --local` still shows the original tag (`v1`) with status `modified` rather than the new tag (`v2`) with status `clean`.

```bash
$ nebi pull data-science:v1
$ pixi add numpy           # modifies pixi.toml and pixi.lock
$ nebi push data-science:v2

$ nebi workspace list --local
# Shows: data-science  v1  modified   ~/project   ← WRONG
# Should: data-science  v2  clean     ~/project   ← CORRECT
```

## Root Cause

In `cmd/nebi/push.go` (lines 147–157), after `client.PublishEnvironment()` returns successfully, the command only prints the result. It does **not**:

1. Update the `.nebi` file with the new origin (new tag, new digest)
2. Update `index.json` with the new workspace entry
3. Recompute layer digests for the current local files

By contrast, `cmd/nebi/pull.go` (lines 218–257) performs all three of these steps after a successful pull.

### Current push.go post-success code:

```go
fmt.Printf("Pushed %s:%s\n", repository, tag)
if resp.Digest != "" {
    fmt.Printf("  Digest: %s\n", resp.Digest)
}
fmt.Printf("\nSuccessfully pushed to %s\n", registry.Name)
// END — no metadata updates
```

### What pull.go does after success:

```go
// 1. Compute layer digests
pixiTomlDigest := nebifile.ComputeDigest(pixiTomlBytes)
pixiLockDigest := nebifile.ComputeDigest(pixiLockBytes)

// 2. Write .nebi metadata file
nf := nebifile.NewFromPull(...)
nebifile.Write(outputDir, nf)

// 3. Update local index
entry := localindex.WorkspaceEntry{...}
idxStore.AddEntry(entry)
```

## What Should Happen After Successful Push

After `client.PublishEnvironment()` returns successfully, push should mirror the post-pull update logic:

### 1. Compute layer digests for current files

The local `pixi.toml` and `pixi.lock` bytes are already loaded at the top of `runPush`. Compute their SHA-256 digests — these represent the "clean" state that was just pushed.

```go
pixiTomlDigest := nebifile.ComputeDigest(pixiTomlContent)
pixiLockDigest := nebifile.ComputeDigest(pixiLockContent)
```

### 2. Update `.nebi` to point to the new origin

```yaml
# After push data-science:v2, .nebi should become:
origin:
  workspace: data-science
  tag: v2                          # ← updated to new tag
  registry_url: ghcr.io/myorg      # ← OCI registry URL (stable, not user-assigned name)
  server_url: https://nebi.example.com
  server_version_id: 55            # ← new version from server response
  manifest_digest: sha256:<new>    # ← digest from PublishResponse
  pulled_at: 2025-01-23T10:00:00Z  # ← "synced at" time

layers:
  pixi.toml:
    digest: sha256:<current_file_hash>   # ← matches local file
    size: 2345
    media_type: application/vnd.pixi.toml.v1+toml
  pixi.lock:
    digest: sha256:<current_file_hash>   # ← matches local file
    size: 45678
    media_type: application/vnd.pixi.lock.v1+yaml
```

This means drift detection will now compare local files against what was just pushed, yielding `clean` status.

### 3. Update `index.json` with new entry

```json
{
  "workspace": "data-science",
  "tag": "v2",
  "registry_url": "ghcr.io/myorg",
  "server_url": "https://nebi.example.com",
  "server_version_id": 55,
  "path": "/home/user/project",
  "is_global": false,
  "pulled_at": "2025-01-23T10:00:00Z",
  "manifest_digest": "sha256:<new>",
  "layers": {
    "pixi.toml": "sha256:<current_file_hash>",
    "pixi.lock": "sha256:<current_file_hash>"
  }
}
```

Since `AddEntry` matches by `path`, this will **replace** the old `v1` entry for the same directory.

## Information Gap: `server_version_id`

The `.nebi` file stores `server_version_id` (the immutable version number used by `nebi diff` to fetch the exact origin content from the server).

### Server returns it, CLI doesn't capture it

**Server-side** `PublicationResponse` (in `internal/api/handlers/environment.go`):
```go
type PublicationResponse struct {
    ID            uuid.UUID `json:"id"`
    VersionNumber int       `json:"version_number"`   // ← WE NEED THIS
    RegistryName  string    `json:"registry_name"`
    RegistryURL   string    `json:"registry_url"`     // ← WE NEED THIS
    Repository    string    `json:"repository"`
    Tag           string    `json:"tag"`
    Digest        string    `json:"digest"`
    PublishedBy   string    `json:"published_by"`
    PublishedAt   string    `json:"published_at"`
}
```

**CLI-side** `PublishResponse` (in `internal/cliclient/types.go`):
```go
type PublishResponse struct {
    Digest     string `json:"digest"`
    Repository string `json:"repository"`
    Tag        string `json:"tag"`
    // MISSING: VersionNumber, RegistryURL
}
```

**Fix**: Add `VersionNumber int32` and `RegistryURL string` to the CLI's `PublishResponse` struct. The JSON fields are already in the server response — just not being deserialized.

## Design Decision: Registry URL vs Registry Name in `.nebi`

### The Problem

The `.nebi` `Origin` struct has a `Registry string` field. The question is: should this store the **user-assigned registry name** (e.g., `ds-team`) or the **registry URL** (e.g., `ghcr.io/myorg`)?

### Why Registry URL is Better

The registry name is a local alias assigned by the user via `nebi registry add <name> <url>`. It is:
- **Mutable**: User can `nebi registry remove ds-team && nebi registry add my-team <same-url>`
- **Local**: Different users may assign different names to the same registry
- **Not unique across machines**: No guarantee another machine uses the same name

The registry URL is:
- **Stable**: The actual OCI registry address doesn't change when the user renames their alias
- **Globally meaningful**: Same URL means same registry regardless of local config
- **Already available**: The server returns `registry_url` in `PublicationResponse`, and the `Registry` struct has `.URL`

### Recommendation

**Store `registry.URL` in the `.nebi` file's `origin.registry` field** (or rename the field to `registry_url` for clarity).

This way, if a user removes and re-adds a registry with a different name, the `.nebi` file still correctly identifies where the artifact lives. Commands like `nebi status` can look up the registry by URL to resolve back to the current local name for display purposes.

### Current State

The pull command currently passes `""` for the registry field:
```go
nf := nebifile.NewFromPull(
    workspaceName, tag, "", cfg.ServerURL,  // ← registry is always empty string
    ...
)
```

So this field is unused today. We can define it correctly now without any migration concerns.

### Proposed `.nebi` Origin Field

```yaml
origin:
  workspace: data-science
  tag: v2
  registry_url: ghcr.io/myorg    # ← stable OCI registry URL
  server_url: https://nebi.example.com
  server_version_id: 55
  manifest_digest: sha256:...
  pulled_at: 2025-01-23T10:00:00Z
```

Note: We may want to rename the struct field from `Registry` to `RegistryURL` and the YAML key from `registry` to `registry_url` for clarity — since the field currently exists but is always empty, there's no migration burden.

## Edge Cases

### Push to new tag (normal case)
- Update `.nebi` origin tag to new tag
- Update index entry (replaces old entry by path)
- Status becomes `clean`

### Push to same tag with `--force`
- Same as new tag: update `.nebi` with new digest, same tag name
- The manifest digest will differ from the previous push (content changed)
- Status becomes `clean`
- Design note: the existing drift warning in `showPushDriftWarning()` already warns about same-tag pushes

### Push from directory without existing `.nebi`
- This is a "fresh push" — directory was never pulled from a registry
- **Should still create** `.nebi` and add to index
- The workspace now has an origin for future drift detection

### Push from directory without existing index entry
- Add a new entry to index (no path collision to replace)
- Normal case for first-ever push from a directory

### Push to a different registry than origin
- Update `.nebi` registry URL to the new registry's URL
- Future `nebi diff --remote` will fetch from the new registry
- Previous origin registry info is lost — this is intentional (you've moved the workspace)

### Push fails (network error, auth, server error)
- Do NOT update `.nebi` or index
- Current behavior already correct: errors exit before the metadata-update code would run

## The `pulled_at` Field Naming

There's a semantic awkwardness: the field is called `pulled_at` but after a push we're setting it to "now". Options:

1. **Rename to `synced_at`**: More accurate, but requires migration of existing `.nebi` files and `index.json`
2. **Keep `pulled_at`**: Accept the misnomer — semantically it means "when was this origin state established"
3. **Add parallel `pushed_at` field**: More complex, unclear which takes precedence

**Recommendation**: Keep `pulled_at` for now. It means "the timestamp at which the local state was known to match the origin." This is true for both pull (got remote state) and push (sent local state). Can be renamed in a future schema version.

## Manifest Digest Semantics

The `resp.Digest` from `PublishEnvironment` is the OCI manifest digest of what was pushed. This is exactly what `.nebi`'s `origin.manifest_digest` should store — it's the immutable content identifier for the pushed version, just like it is for a pulled version.

The layer digests can be computed locally since `pixiTomlContent` and `pixiLockContent` are already in memory at that point in the push flow. The local SHA-256 matches the OCI layer digest because Nebi's OCI publisher stores files as raw blobs (not tar archives).

## Implementation Checklist

1. [ ] **Expand `PublishResponse`** in `internal/cliclient/types.go`:
   - Add `VersionNumber int32 \`json:"version_number"\``
   - Add `RegistryURL string \`json:"registry_url"\``

2. [ ] **Rename `Origin.Registry` to `Origin.RegistryURL`** in `internal/nebifile/nebifile.go`:
   - Change struct field from `Registry string \`yaml:"registry,omitempty"\`` to `RegistryURL string \`yaml:"registry_url,omitempty"\``
   - Update any code that references `nf.Origin.Registry` (status.go, helpers.go)
   - No migration needed since the field is currently always empty

3. [ ] **Add post-push metadata update** in `cmd/nebi/push.go` after the success print:
   - Compute `pixiTomlDigest` and `pixiLockDigest` using `nebifile.ComputeDigest()`
   - Construct `NebiFile` with new origin info (workspace, tag, registry URL, server_url, version_number, digest)
   - Write via `nebifile.Write(absDir, nf)`
   - Construct `localindex.WorkspaceEntry` and call `idxStore.AddEntry(entry)`

4. [ ] **Add imports** to `push.go`: `localindex` package, `time` (already imported)

5. [ ] **Handle the "no pixi.lock" case**: If `pixiLockContent` is empty (warning was shown earlier), skip the lock layer in `.nebi` layers map or use empty digest

6. [ ] **Also update pull** to populate `RegistryURL` (currently passes `""` — should use registry URL when available)

7. [ ] **Test scenarios**:
   - Push to new tag → verify `.nebi` updated, `workspace list` shows new tag as clean
   - Push to same tag → verify `.nebi` digest updated
   - Push from fresh directory (no `.nebi`) → verify `.nebi` created
   - Push failure → verify `.nebi` unchanged
   - Registry rename → verify `.nebi` still works (URL unchanged)

## Files to Modify

| File | Change |
|------|--------|
| `internal/cliclient/types.go` | Add `VersionNumber` and `RegistryURL` to `PublishResponse` |
| `internal/nebifile/nebifile.go` | Rename `Origin.Registry` → `Origin.RegistryURL`, update YAML tag |
| `internal/localindex/localindex.go` | Rename `WorkspaceEntry.Registry` → `RegistryURL`, update JSON tag |
| `cmd/nebi/push.go` | Add post-push `.nebi` + index update logic after line 157 |
| `cmd/nebi/pull.go` | Populate `RegistryURL` field (currently passes `""`) |
| `cmd/nebi/status.go` | Update references from `.Registry` to `.RegistryURL` |
| `cmd/nebi/helpers.go` | Update references from `.Registry` to `.RegistryURL` |

## Relationship to Design Docs

From `duplicate-pulls.md` (line 549):
> ```
> # The .nebi file is updated to reflect new origin
> ```

This behavior was always intended by the design but never implemented. The design doc explicitly shows the `.nebi` file being updated after push — this issue is simply a gap between spec and implementation.
