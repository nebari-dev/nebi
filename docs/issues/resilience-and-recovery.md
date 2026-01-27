# Resilience and Recovery Scenarios

Covers: `.nebi` file loss, server redeployment, API versioning, connected-server visibility, and a future repair command.

---

## 1. `.nebi` File Deleted or Lost

### Impact per Command

| Command | Impact | Current Behavior |
|---------|--------|-----------------|
| `nebi status` | **Broken** | Exit 2: `"not a nebi workspace: .nebi not found"` + hint to `nebi pull` |
| `nebi status --remote` | **Broken** | Same (reads .nebi before any remote check) |
| `nebi diff` | **Broken** | Same error + hint |
| `nebi diff --remote` | **Broken** | Same |
| `nebi diff ./a ./b` | **Works** | Path-vs-path mode reads pixi.toml directly, no .nebi needed |
| `nebi shell` (no args, in dir) | **Degrades** | Falls back to checking for `pixi.toml`; still shells but no drift warning |
| `nebi shell ws:tag` | **Works** | Resolves via local index, not .nebi |
| `nebi push` | **Mostly works** | Core push works (reads pixi.toml/lock from disk); skips drift warning and post-push .nebi update |
| `nebi push --dry-run` | **Degrades** | Shows file sizes instead of diff: `"(No .nebi metadata found)"` |
| `nebi workspace list --local` | **Partially works** | Lists from index; drift check returns `"unknown"` for entries without .nebi |
| `nebi workspace info` (no args) | **Broken** | `"Not a nebi workspace directory (no .nebi file found)"` |
| `nebi workspace info name` | **Works** | Server lookup only, no .nebi needed |
| `nebi workspace prune` | **Works** | Checks path existence only |
| `nebi pull` | **Works** | Re-creates .nebi (this IS the recovery path) |
| `nebi pull` (already up to date check) | **Skipped** | `checkAlreadyUpToDate` returns false when .nebi missing, so pull proceeds normally |

### Key Observation

The local index (`~/.local/share/nebi/index.json`) stores redundant copies of all .nebi data:
- `workspace`, `tag`, `server_url`, `server_version_id`
- `manifest_digest`, `layers` (digest map)
- `pulled_at`, `registry_url`

This means a missing `.nebi` can be **regenerated** from the index if the local files still match the stored layer digests.

### Proposed Solution

Add a `nebi repair` subcommand (or integrate into existing commands):

```
nebi repair [--path DIR]
```

Logic:
1. Check if `.nebi` exists in DIR. If yes, validate it (parseable YAML, layers match files).
2. If missing: compute SHA256 of local `pixi.toml` and `pixi.lock`.
3. Search local index for entries whose layer digests match AND whose path matches (or whose path is stale but digests match).
4. If a unique match is found: regenerate `.nebi` from the index entry.
5. If multiple matches: present them and ask user to confirm.
6. If no match: suggest `nebi pull --force` to re-download.

Alternatively, commands like `nebi status` and `nebi diff` could auto-detect and offer to repair:
```
Error: .nebi not found
However, local files match index entry for data-science:v1 (pulled 2h ago).
Run 'nebi repair' to restore tracking metadata.
```

---

## 2. Server Redeployment (DB Erased)

The `.nebi` file stores `server_version_id` (e.g., 42) and `server_url`. If the server is redeployed with a fresh database, those version IDs become dangling references.

### Impact per Command

| Scenario | What Happens |
|----------|-------------|
| `nebi status` (offline) | **Works** -- only compares local hashes against .nebi layer digests |
| `nebi status --remote` | `CheckRemote()` lists envs by name. If workspace gone: `RemoteStatus.Error = "workspace 'X' not found on server"`. Output: `"clean locally (remote check failed)"` |
| `nebi diff` (local vs origin) | Calls `GET /environments/{id}/versions/42/pixi-toml`. Server returns 404. CLI: `"Error: Failed to fetch origin content"` + `"Hint: The origin version may no longer be available on the server."` |
| `nebi diff --remote` | Looks up workspace by name, then tag. If workspace missing: error. If tag missing: `"tag 'v1' not found for workspace"` |
| `nebi push` | Creates a new workspace (fresh DB, name available) -- works but generates a new version ID |
| `nebi shell` (local) | **Works** -- pixi shell runs locally |
| `nebi workspace list` | Returns empty (fresh DB) or different workspaces |
| `nebi workspace info` (no args) | Local section works; Server section shows `"workspace not found on server"` |

### Root Cause

`nebi diff` (without `--remote`) fetches origin content by `server_version_id`. This is an opaque integer that only has meaning within a specific server database instance. When the DB is wiped, version 42 no longer exists.

The `manifest_digest` (OCI content-addressed hash) is more durable -- it survives as long as the OCI registry storage isn't wiped. But the CLI doesn't currently use it for content retrieval.

### Proposed Solutions

#### A. Graceful degradation with clear messaging (quick fix)

When `FetchVersionContent` returns a 404, detect it and provide actionable guidance:

```
Error: Origin version 42 not found on server.
The server may have been redeployed. Your local files are intact.

Options:
  nebi diff --remote          Compare against current remote tag instead
  nebi pull --force           Re-pull from server (overwrites local changes)
  nebi push data-science:v1   Re-publish your local version to the new server
```

#### B. Content-addressable fallback (medium-term)

If `server_version_id` fails with 404:
1. Check if the `manifest_digest` from .nebi can resolve via the OCI registry directly.
2. If the OCI registry still has the content, fetch layers by digest instead of version ID.
3. This decouples `nebi diff` from the server database entirely for origin comparisons.

This requires adding a registry-based content fetch path alongside the current server-based one.

#### C. Local content cache (best long-term)

Store the original `pixi.toml` and `pixi.lock` content locally at pull time in a content-addressable cache:

```
~/.local/share/nebi/blobs/sha256/111...  (original pixi.toml content)
~/.local/share/nebi/blobs/sha256/222...  (original pixi.lock content)
```

Benefits:
- `nebi diff` works fully offline (no server fetch needed)
- Survives server redeployment
- Tiny storage cost (pixi.toml ~1KB, pixi.lock ~20-50KB each)
- Layer digests in .nebi already point to the right blobs

This makes `nebi diff` (local vs origin) a purely offline operation, with the server only needed for `--remote` comparisons.

#### D. `nebi reattach` command (for server migration)

When a server is redeployed and the user re-pushes their workspace:

```
nebi reattach [--path DIR]
```

Logic:
1. Read .nebi to get workspace name and tag.
2. Look up the workspace on the currently-configured server.
3. Find the version whose content matches the stored layer digests.
4. Update .nebi with the new `server_version_id`.
5. Update the local index entry.

---

## 3. Server API Versioning

### Current State

- All routes live under `/api/v1`.
- `GET /api/v1/version` exists but the CLI never checks it.
- No version negotiation between CLI and server.
- Go's JSON unmarshalling silently ignores unknown fields (additions are safe).

### Compatibility Risks

| Change Type | Risk |
|-------------|------|
| Adding response fields | Safe -- old CLIs ignore them |
| Removing response fields | **Breaking** -- old CLIs may panic on nil/zero |
| Adding new endpoints | Safe -- old CLIs won't call them |
| Changing endpoint behavior | **Breaking** if semantics change |
| Renaming fields | **Breaking** |
| Changing auth mechanism | **Breaking** |

### Proposed Solution: Version Handshake

#### Step 1: Server advertises requirements

Add to the `GET /api/v1/version` response:

```json
{
  "server_version": "0.5.0",
  "api_version": "v1",
  "minimum_cli_version": "0.3.0",
  "deprecated_since": null
}
```

#### Step 2: CLI checks on login

After `nebi login`, call `/api/v1/version` and compare `minimum_cli_version` against the CLI's compiled-in version. Warn (don't block) if outdated:

```
Warning: This server requires CLI version >= 0.3.0 (you have 0.2.1)
Some features may not work correctly. Run 'nebi upgrade' to update.
```

#### Step 3: Periodic check

On commands that talk to the server (`push`, `pull`, `status --remote`, `diff --remote`), perform a lightweight version check (cached for 24h) and warn if outdated.

#### Step 4: Multi-version serving (future)

When a breaking change is needed:
- Server serves both `/api/v1` and `/api/v2` during a transition period.
- CLI's `loadConfig()` stores `api_version` preference.
- Old CLIs continue using `/api/v1` until deprecated.
- Deprecation: server returns `Sunset` header or 299 warning code on `/api/v1` responses.

---

## 4. CLI Command to Show Connected Server

### Where Server URL Lives

| Location | Field | Purpose |
|----------|-------|---------|
| `~/.config/nebi/config.yaml` | `server_url` | Currently active connection |
| `.nebi` (per workspace) | `origin.server_url` | Where THIS workspace was pulled from |
| `~/.local/share/nebi/index.json` | `server_url` per entry | Origin server for each tracked workspace |

The active connection (config.yaml) may differ from a workspace's origin server. This is currently invisible to the user.

### Proposed: `nebi server` Command

```
$ nebi server
Connected to: https://nebi.example.com
User:          alice
Status:        authenticated

$ nebi server --verbose
Connected to: https://nebi.example.com
User:          alice
Token:         eyJhb...Kw (expires 2026-02-15)
API version:   v1
Server:        nebi 0.5.0
Config file:   ~/.config/nebi/config.yaml
```

Implementation:
1. Read `~/.config/nebi/config.yaml` for `server_url` and token presence.
2. Optionally call `GET /api/v1/auth/me` to confirm user identity.
3. Optionally call `GET /api/v1/version` for server version info.
4. Show warning if the current workspace's .nebi `server_url` differs from config.

#### Mismatch Warning

When in a workspace directory whose `.nebi` server_url differs from the active config:

```
$ nebi server
Connected to: https://nebi-prod.example.com
User:          alice

Note: This workspace was pulled from https://nebi-staging.example.com
      Remote commands (diff --remote, status --remote) use the connected server.
```

This helps users understand why `nebi diff --remote` might give unexpected results when switching between servers.

---

## 5. `nebi repair` Command (Future)

### Feasibility

**Hardlink context:** Nebi's pull uses `os.WriteFile()` which creates independent
copies of `pixi.toml` and `pixi.lock` — no hardlinks between nebi workspaces.
However, **pixi itself** uses hardlinks between its global package cache
(`~/.cache/rattler/`) and the per-workspace `.pixi/envs/` directory (at least on
Linux). This means:

- The pixi.toml/pixi.lock files that nebi manages are small regular files (not
  hardlinked) — inode-based search won't help find moved nebi workspaces.
- The large environment contents in `.pixi/` ARE hardlinked to the pixi cache,
  so moving a workspace directory preserves those links (as long as it stays on
  the same filesystem). This means `nebi repair --scan` finding a moved workspace
  is still useful — the expensive `.pixi/` install is preserved via hardlinks
  even after the directory moves.

**Inode-based search for nebi files specifically:** Not useful since pixi.toml and
pixi.lock are independent copies. Would require nebi to adopt a content-addressable
store with hardlinks for the manifest files themselves, which adds complexity for
files that are only ~1-50KB each.

**Digest-based search (practical today):** The layer digests (SHA256 of file
content) stored in the local index provide an equivalent lookup mechanism that
works regardless of filesystem type and doesn't require hardlinks.

### Proposed Design

```
nebi repair [--scan] [--path DIR]
```

#### Mode 1: Index-based repair (default, fast)

```
$ nebi repair
Checking workspace at /home/user/project-a...
  .nebi file: missing
  pixi.toml:  sha256:f9f911... (257 bytes)
  pixi.lock:  sha256:2127ca... (18588 bytes)

Found matching index entry:
  data-science:v1 (pulled 2h ago from https://nebi.example.com)

Regenerate .nebi? [Y/n]: y
Restored .nebi metadata for data-science:v1
```

Logic:
1. Compute digests of local pixi.toml and pixi.lock.
2. Search index entries where ALL layer digests match.
3. If unique match found: regenerate .nebi.
4. If multiple matches: prompt user to select.
5. If no match: files have been modified since pull, can't auto-repair.

#### Mode 2: Filesystem scan (--scan, slow)

```
$ nebi repair --scan
Scanning for nebi workspaces...
  /home/user/old-project/  pixi.toml matches data-science:v1 (index says /home/user/project-a/)
  Workspace appears to have moved.

Update index entry? [Y/n]: y
Updated path: /home/user/project-a/ -> /home/user/old-project/
Regenerated .nebi
```

Logic:
1. Walk common directories (`~`, or a configurable search path).
2. Find all `pixi.toml` files.
3. Compute their digests.
4. Match against known layer digests in the index.
5. Offer to update index entries and regenerate .nebi files.

This is expensive (full filesystem walk) so it should be opt-in and show progress.

**Pixi hardlink bonus:** When a workspace is moved (not copied) within the same
filesystem, pixi's hardlinks from `.pixi/envs/` to the global cache survive. So
finding a moved workspace via `--scan` means the user doesn't need to re-run
`pixi install` — the environment is already intact. This makes repair genuinely
useful rather than just a shortcut for re-pulling.

#### Mode 3: Index repair (--index)

```
$ nebi repair --index
Checking 5 index entries...
  data-science:v1 at /home/user/project-a/  OK
  ml-pipeline:v2 at /home/user/ml/          MISSING (directory gone)
  web-app:latest at /home/user/webapp/       STALE (.nebi digest mismatch)

Remove 1 missing entry? [Y/n]: y
Removed ml-pipeline:v2 (was at /home/user/ml/)
```

This is a more interactive version of `nebi workspace prune` that also validates .nebi file integrity.

### UX Considerations

- `nebi repair` with no flags should be safe and fast (index lookup only).
- `--scan` should show a progress indicator and allow Ctrl+C.
- Never modify files without user confirmation (unless `--yes` is passed).
- Exit codes: 0 = repaired or nothing to repair, 1 = unrepairable (suggest pull), 2 = error.
- Works offline (index + local file hashes only, no server needed).

---

## Summary: Priority and Complexity

| Issue | Solution | Priority | Complexity |
|-------|----------|----------|-----------|
| `.nebi` deleted | `nebi repair` from index | High | Low |
| Server redeployed (diff broken) | Better error message + suggest `--remote` | High | Low |
| Server redeployed (long-term) | Local content cache (blobs/) | Medium | Medium |
| API versioning | Version check on login, warn if outdated | Medium | Low |
| Show connected server | `nebi server` command | High | Low |
| Server mismatch warning | Compare .nebi server_url vs config | Medium | Low |
| Repair: filesystem scan | `nebi repair --scan` (digest-based, not inode) | Low | Medium |
| `nebi reattach` | Re-link workspace to new server | Low | Medium |

### Recommended Implementation Order

1. **`nebi server`** -- simple, high visibility, builds user confidence
2. **Better error messages** for stale version IDs (already has the hint text, just improve it)
3. **`nebi repair`** (index-based mode only) -- covers the most common accidental deletion case
4. **Version check on login** -- low effort, prevents confusion from version skew
5. **Local content cache** -- unlocks fully offline diff and survives server redeployments
6. **`nebi repair --scan`** -- nice to have, expensive to implement well
