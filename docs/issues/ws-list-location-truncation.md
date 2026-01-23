# Issue: `nebi ws list` Location Column Truncation

## Problem

The `LOCATION` column in `nebi ws list --local` truncates global workspace paths,
making them indistinguishable:

```
$ nebi ws list --local
WORKSPACE     TAG  STATUS  LOCATION
data-science  v1   clean   ~/eph/nebi/my-pixi-env (local)
data-science  v2   clean   ~/.local/share/nebi/... (global)
data-science  v3   clean   ~/.local/share/nebi/... (global)   <-- same as above!
```

All global paths are rendered as `~/.local/share/nebi/... (global)` regardless of
which UUID/tag directory they actually point to. This is semantically destructive --
you cannot tell which global workspace is which from the location alone.

## Root Cause

In `cmd/nebi/workspace.go`, the `formatLocation()` function unconditionally truncates
any path under `~/.local/share/nebi/`:

```go
if isGlobal {
    dataDir := filepath.Join(home, ".local", "share", "nebi")
    if strings.HasPrefix(path, dataDir) {
        display = "~/.local/share/nebi/..." + " (global)"
        return display
    }
}
```

## Research: How Other CLI Tools Handle This

| Tool | Default Behavior | Width Control | JSON Support | Uses `~`? |
|------|-----------------|---------------|--------------|-----------|
| `docker ps` | Truncates (IDs, commands) | `--no-trunc` | `--format json` | No |
| `kubectl get` | No truncation (columns expand) | `-o wide` adds columns | `-o json/yaml` | No |
| `git branch -v` | No truncation | `--no-abbrev` for hashes | No | No |
| `conda env list` | No truncation (full absolute paths) | None | `--json` | No |
| `helm list` | Truncates (~50 chars) | `--max-col-width N` (0=unlimited) | `-o json/yaml` | No |
| `pip list` | No truncation (auto-expand) | None | `--format json` | No |

Key findings:
1. Only 2 of 7 tools truncate by default (docker, helm). Most just let columns expand.
2. No tool uses `~` abbreviation -- they all show full absolute paths.
3. JSON output is nearly universal as the escape hatch for scripts.
4. The helm approach (`--max-col-width` with 0=unlimited) is the most nuanced but
   over-engineered for nebi's simpler use case.

## Options Considered

| Option | Pros | Cons |
|--------|------|------|
| A: `--wide` flag | Familiar pattern; default stays compact | Extra flag; doesn't fix the real problem |
| B: Never truncate | Simple; always accurate | Can get wide on narrow terminals |
| C: Truncate from left | Shows useful end of path | Unfamiliar; still lossy |
| D: Tilde-shorten, never `...` | Good balance; lossless | Long paths still wide |
| E: `--json` output mode | Perfect for scripts | Not human-readable; complementary |
| F: `--max-col-width` (helm-style) | Most flexible | Over-engineered for this use case |

## Decision: Option D + E

Tilde-shorten but never truncate with `...`, plus add `--json` for scripting.

### Rationale

1. **The `...` truncation is the bug, not the width.** The problem isn't that we need
   a `--wide` mode -- it's that the default mode destroys information. All global paths
   look identical. Fixing the default is the right answer.

2. **Tilde (`~`) shortening is a lossless abbreviation.** It's universally understood
   and reversible. `...` truncation is lossy and makes entries indistinguishable.

3. **`tabwriter` handles alignment.** Go's `text/tabwriter` (already in use) will
   auto-align columns regardless of path length. Let it do its job.

4. **Terminal widths are generous.** Most terminals today are 120+ characters.
   `~/.local/share/nebi/workspaces/<uuid>/v1.0` is typically ~65 chars -- reasonable.

5. **UUID abbreviation for display.** Like git abbreviates commit hashes, we can show
   only the first 8 characters of UUIDs in human-readable output (full path in `--json`).

6. **`--json` is already established in nebi.** Both `nebi status --json` and
   `nebi diff --json` exist. Adding it to `ws list --local` is consistent.

### Why NOT `--wide` or `--no-trunc`

Docker's `--no-trunc` exists because container IDs are inherently long hex strings that
are rarely useful in full. Our paths are meaningful human-readable strings that users
need to distinguish between entries. The fix belongs in the default behavior.

## Implementation

### Display output (default)

```
$ nebi ws list --local
WORKSPACE     TAG   STATUS    LOCATION
data-science  v1.0  clean     ~/.local/share/nebi/workspaces/550e8400/v1.0 (global)
data-science  v1.0  modified  ~/project-a (local)
ml-tools      v2.0  clean     ~/work/ml-project (local)
```

- UUIDs abbreviated to first 8 characters in display
- Tilde shortening applied
- `(local)` / `(global)` suffix preserved
- No `...` truncation ever

### JSON output (`--json`)

```bash
$ nebi ws list --local --json
```

```json
[
  {
    "workspace": "data-science",
    "tag": "v1.0",
    "status": "clean",
    "path": "/home/user/.local/share/nebi/workspaces/550e8400-e29b-41d4-a716-446655440000/v1.0",
    "is_global": true
  },
  {
    "workspace": "data-science",
    "tag": "v1.0",
    "status": "modified",
    "path": "/home/user/project-a",
    "is_global": false
  }
]
```

- Full absolute paths (no tilde, no abbreviation)
- Structured data for script consumption
- Consistent with existing `--json` flags in nebi

### Changes Required

1. `cmd/nebi/workspace.go`:
   - Modify `formatLocation()` to show full tilde-shortened path (no `...`)
   - Abbreviate UUIDs (first 8 chars) in global paths for display
   - Add `--json` flag to `workspaceListCmd`
   - Add JSON output path in `runWorkspaceListLocal()`

2. `cmd/nebi/workspace_test.go`:
   - Update `TestFormatLocation_Global` to expect full path with abbreviated UUID
   - Add test for JSON output
