# Rename "workspace" to "repo" + Split push/publish

## Motivation

Nebi uses the term "workspace" for its central concept — the named entity that holds tagged versions of pixi.toml/pixi.lock files. This creates a naming collision with Pixi itself, which uses `[workspace]` as a section in pixi.toml (with its own `name` field).

The confusion is compounded by the fact that these two "workspace" names are **completely independent**. You can `nebi push foo:v1.0` and the pixi.toml inside can have `[workspace] name = "bar"`. This is already the case in the existing data — e.g., the environment named `my-ds-env` contains a pixi.toml with `[workspace] name = "my-pixi-env"`.

### Why "repo"?

Nebi's architecture is modeled directly on OCI registries (Docker Hub, ghcr.io, etc.):
- `nebi push data-science:v1.0` is analogous to `docker push myorg/my-image:v1.0`
- The named entity (`data-science`) functions as an **OCI repository** — a collection of tagged manifests
- Tags, digests, registries, and push/pull semantics all follow OCI conventions
- The server's `Publication` model already has a field called `Repository`

In OCI terminology, a **repository** is "a scope for a collection of content including manifests, blobs, and tags." That's exactly what a Nebi "workspace" is. Calling it "repo" aligns with the underlying model and eliminates the Pixi naming conflict.

### Why split push/publish?

Currently `nebi push` does three things in one operation:
1. Creates/finds the Environment on the Nebi server
2. Creates an EnvironmentVersion (stores pixi.toml + pixi.lock)
3. Publishes to an OCI registry

But `nebi pull` already fetches directly from the Nebi server's database — it never touches the OCI registry. This means the OCI publish step is a **distribution concern**, not a storage concern. Separating them:
- Makes OCI registries optional (useful during initial setup, air-gapped envs, or simple team use)
- Enables review-before-distribute workflows (push a version, review, then publish)
- Enables multi-registry publishing (push once, publish to team + company registries)
- Matches the Helm model (`helm package` then `helm push`) and Docker model (`docker build` then `docker push`)
- Simplifies onboarding (new users can push without configuring OCI registries)

### Decisions Made

- **Tag required on push**: `nebi push data-science:v1.0` assigns the tag when content goes to server. Publishing distributes an already-tagged version.
- **Pull is server-first**: Already the case — pull fetches from the Nebi server DB. OCI registries are a distribution layer, not the pull source.
- **Clean break on rename**: `nebi repo` only, no `ws` or `workspace` aliases.

---

## Implementation Plan

### Overview

Two major changes:
1. **Rename "workspace" to "repo"** in CLI commands, help text, local storage field names, and .nebi metadata
2. **Split `push` into `push` (server-only) + `publish` (OCI registry)**

The server-side "environment" model/API stays unchanged (internal implementation detail).

---

## Part 1: Rename workspace to repo

### 1.1 CLI Command Rename

**File: `cmd/nebi/workspace.go` -> rename to `cmd/nebi/repo.go`**

- `workspaceCmd` -> `repoCmd` (Use: "repo", no aliases -- clean break)
- `workspaceListCmd` -> `repoListCmd`
- `workspaceListLocal` -> `repoListLocal`
- `workspaceListJSON` -> `repoListJSON`
- `workspacePruneCmd` -> `repoPruneCmd`
- `workspaceTagsCmd` -> `repoTagsCmd`
- `workspaceDeleteCmd` -> `repoDeleteCmd`
- `workspaceInfoCmd` -> `repoInfoCmd`
- `workspaceDiffCmd` -> `repoDiffCmd`
- All help text: s/workspace/repo/g, s/Workspace/Repo/g
- Remove `Aliases: []string{"ws"}` -- no backward compat

**File: `cmd/nebi/main.go`**

- `rootCmd.AddCommand(workspaceCmd)` -> `rootCmd.AddCommand(repoCmd)`
- Update `Long` description: all references to "workspace" -> "repo"

**File: `cmd/nebi/push.go`**

- Help text: `<workspace>:<tag>` -> `<repo>:<tag>`, "workspace" -> "repo" in descriptions
- `parseWorkspaceRef` -> `parseRepoRef` (and update all callers)
- Variable `workspaceName` -> `repoName` in `runPush`
- String literals: "Creating workspace" -> "Creating repo", etc.
- `findWorkspaceByName` -> `findRepoByName` (and update all callers)
- `showPushDriftWarning` messages: "workspace" -> "repo"

**File: `cmd/nebi/pull.go`**

- Help text: `<workspace>[:<tag>]` -> `<repo>[:<tag>]`
- "Pull workspace from server" -> "Pull repo from server"
- Variable `workspaceName` -> `repoName` in `runPull`
- `handleGlobalPull` / `handleDirectoryPull`: parameter name `workspace` -> `repo`
- `checkAlreadyUpToDate`: parameter name, messages
- `pullGlobal` path description: "workspaces" -> "repos" in help text

**File: `cmd/nebi/shell.go`**

- Help text: "workspace" -> "repo" references
- Internal variable names

**File: `cmd/nebi/status.go`**

- Messages like "Workspace:" -> "Repo:" in output

**File: `cmd/nebi/diff.go`**

- Messages referencing "workspace" in output

### 1.2 Local Index Rename

**File: `internal/localindex/localindex.go`**

- `WorkspaceEntry` -> `RepoEntry`
- `WorkspaceEntry.Workspace` field -> `RepoEntry.Repo` (JSON tag: `"repo"`)
- `Index.Workspaces` -> `Index.Repos` (JSON tag: `"repos"`)
- `FindByWorkspaceTag` -> `FindByRepoTag`
- `GlobalWorkspacePath` -> `GlobalRepoPath`
- All comments and string literals
- **Migration**: Add backward-compat JSON reading -- if `"workspaces"` key exists in index.json, read it into `Repos`. Write always uses new format. (One-time migration on first load.)

### 1.3 Nebifile Rename

**File: `internal/nebifile/nebifile.go`**

- `Origin.Workspace` -> `Origin.Repo` (YAML tag: `"repo"`)
- `NewFromPull` parameter name: `workspace` -> `repo`
- **Migration**: When reading .nebi files, accept both `workspace:` and `repo:` YAML keys. Write always uses `repo:`. (Add a custom YAML unmarshaler or post-read fixup.)

### 1.4 Drift Package Rename

**File: `internal/drift/drift.go`**

- `WorkspaceStatus` -> `RepoStatus`
- Comments and error messages

**File: `internal/drift/remote.go`**

- "workspace %q not found" -> "repo %q not found"
- Variable/parameter names

### 1.5 Test Files

Update all test files referencing workspace:
- `cmd/nebi/workspace_test.go` -> `cmd/nebi/repo_test.go`
- `cmd/nebi/push_test.go` -- variable names, string assertions
- `cmd/nebi/pull_test.go` -- same
- `cmd/nebi/shell_test.go` -- same
- `internal/localindex/localindex_test.go`
- `internal/nebifile/nebifile_test.go`
- `internal/drift/*_test.go`

---

## Part 2: Split push into push + publish

### 2.1 New Server API Endpoint: Push (Create Version + Tag)

**File: `internal/api/handlers/environment.go`**

Add new handler `PushVersion`:
```
POST /environments/:id/push
Body: { "tag": "v1.0", "pixi_toml": "...", "pixi_lock": "..." }
Response: { "version_number": 3, "tag": "v1.0" }
```

This does what lines 969-1000 of `PublishEnvironment` currently do:
- Write pixi.toml/pixi.lock to envPath
- Create `EnvironmentVersion` record
- Assign a server-side tag (new model, see 2.2)
- Return the version number

**File: `internal/api/router.go`**

Add route:
```go
env.POST("/push", middleware.RequireEnvironmentAccess("write"), envHandler.PushVersion)
```

### 2.2 New Server Model: EnvironmentTag

**File: `internal/models/environment_tag.go` (new file)**

```go
type EnvironmentTag struct {
    ID              uuid.UUID   `gorm:"type:text;primary_key"`
    EnvironmentID   uuid.UUID   `gorm:"type:text;not null;index:idx_env_tag,unique"`
    Tag             string      `gorm:"not null;index:idx_env_tag,unique"`
    VersionNumber   int         `gorm:"not null"`
    CreatedBy       uuid.UUID   `gorm:"type:text;not null"`
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

Unique constraint on (EnvironmentID, Tag) -- one tag per name per repo.
Tags are mutable (can be re-pointed to a new version on subsequent push).

### 2.3 Server: Refactor PublishEnvironment

Modify `PublishEnvironment` to:
- Accept version reference instead of raw content
- Look up version by tag or version_number
- Only do the OCI publish part (lines 1011-1066 of current code)

New request format:
```
POST /environments/:id/publish
Body: { "registry_id": "...", "repository": "...", "tag": "v1.0", "version": 3 }
```

Where `version` is the version_number to publish. If not provided, resolve from tag.

Keep backward compat: if `pixi_toml` is provided in publish request, do the old combined behavior (push + publish in one call).

### 2.4 New Client Method: PushVersion

**File: `internal/cliclient/environments.go`**

```go
func (c *Client) PushVersion(ctx, envID string, req PushRequest) (*PushResponse, error)
```

**File: `internal/cliclient/types.go`**

```go
type PushRequest struct {
    Tag      string `json:"tag"`
    PixiToml string `json:"pixi_toml"`
    PixiLock string `json:"pixi_lock,omitempty"`
}

type PushResponse struct {
    VersionNumber int32  `json:"version_number"`
    Tag           string `json:"tag"`
}
```

### 2.5 New CLI Command: `nebi publish`

**File: `cmd/nebi/publish.go` (new file)**

```
nebi publish <repo>:<tag> -r <registry> [--as <oci-repo-name>]
```

- Resolves the repo on the server
- Resolves the tag to a version number (via server-side tag)
- Calls `PublishEnvironment` with the version number + registry info
- `--as` allows publishing under a different OCI repository name (optional)

### 2.6 Modify CLI `push` Command

**File: `cmd/nebi/push.go`**

Change `runPush` to:
1. Find/create the repo (Environment) on server -- same as today
2. Call new `PushVersion` API (creates version + assigns tag) -- replaces the combined publish call
3. Update .nebi metadata and local index -- same as today but without registry fields
4. Print success: "Pushed data-science:v1.0 (version 3)"
5. Hint: "Run 'nebi publish data-science:v1.0 -r <registry>' to distribute via OCI"

**Remove from push:**
- All registry-related logic (finding registry, `-r` flag)
- The `PublishEnvironment` call
- Registry URL from .nebi metadata (it's no longer set on push)

**Keep on push:**
- `--dry-run` (shows diff against origin)
- Environment creation if not exists
- .nebi and local index updates

### 2.7 Update .nebi Metadata

The `.nebi` file after a push (without publish) should look like:
```yaml
origin:
  repo: data-science
  tag: v1.0
  server_url: https://nebi.example.com
  server_version_id: 3
  pulled_at: 2025-01-20T10:30:00Z
layers:
  pixi.toml:
    digest: sha256:...
    size: 2345
    media_type: application/vnd.pixi.toml.v1+toml
```

Note: no `registry_url` or `manifest_digest` -- those come from publish.

After publish, these fields could optionally be updated, but it's not strictly necessary since pull already goes through the server.

### 2.8 Update Local Index

Similar: `RepoEntry` after push has:
- `RegistryURL`: empty (not yet published)
- `ManifestDigest`: empty (OCI digest comes from publish)

After publish, these could be updated but aren't required for pull.

### 2.9 Pull stays the same

Pull already fetches from the Nebi server (via `GetVersionPixiToml`/`GetVersionPixiLock`). The only change is how it resolves tags:

Currently: looks up tag in `Publications` list
After: looks up tag in new `EnvironmentTag` model (server-side tags)

Fallback: if tag not found in server tags, also check publications (backward compat with pre-push/publish-split data).

---

## Part 3: Migration & Backward Compatibility

### 3.1 Local Index Migration

On `Load()`:
- If JSON has `"workspaces"` key, read into `Repos` field
- If entries have `"workspace"` field, map to `Repo`
- On next `Save()`, write new format

### 3.2 .nebi File Migration

On `Read()`:
- Accept both `workspace:` and `repo:` in YAML
- On next `Write()`, use new format

### 3.3 Server API Backward Compat

- Old `POST /environments/:id/publish` with `pixi_toml` in body still works (does push+publish combined)
- New `POST /environments/:id/push` is the preferred path
- New `POST /environments/:id/publish` without `pixi_toml` expects a `version` field

### 3.4 Database Migration

Add `environment_tags` table via GORM AutoMigrate (already the pattern used in the codebase).

---

## Execution Order

1. **Part 1 first** (rename workspace -> repo) -- purely mechanical, no behavior change
2. **Part 2.2** -- Add EnvironmentTag model + migration
3. **Part 2.1** -- Add server push endpoint
4. **Part 2.4** -- Add client PushVersion method
5. **Part 2.6** -- Modify CLI push to use new endpoint (remove registry logic)
6. **Part 2.3** -- Refactor server publish endpoint
7. **Part 2.5** -- Add CLI publish command
8. **Part 2.9** -- Update pull to resolve server-side tags
9. **Part 3** -- Migration/compat (can be done incrementally with each step)

---

## Files Changed (Summary)

### Renamed:
- `cmd/nebi/workspace.go` -> `cmd/nebi/repo.go`
- `cmd/nebi/workspace_test.go` -> `cmd/nebi/repo_test.go`

### New:
- `cmd/nebi/publish.go`
- `internal/models/environment_tag.go`

### Modified:
- `cmd/nebi/main.go`
- `cmd/nebi/push.go`
- `cmd/nebi/pull.go`
- `cmd/nebi/shell.go`
- `cmd/nebi/status.go`
- `cmd/nebi/diff.go`
- `cmd/nebi/push_test.go`
- `cmd/nebi/pull_test.go`
- `cmd/nebi/shell_test.go`
- `internal/localindex/localindex.go`
- `internal/localindex/localindex_test.go`
- `internal/nebifile/nebifile.go`
- `internal/nebifile/nebifile_test.go`
- `internal/drift/drift.go`
- `internal/drift/remote.go`
- `internal/drift/drift_test.go`
- `internal/drift/remote_test.go`
- `internal/cliclient/types.go`
- `internal/cliclient/environments.go`
- `internal/api/router.go`
- `internal/api/handlers/environment.go`
- `internal/api/handlers/publish_test.go`
