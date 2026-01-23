# Feature: Diff Two Local Workspace Paths

## Problem Statement

The `nebi diff` command currently supports comparing:
- Local vs pulled origin (`nebi diff`)
- Local vs current remote (`nebi diff --remote`)
- Two registry references (`nebi diff ws:v1.0 ws:v2.0`)
- Registry ref vs local (`nebi diff ws:v1.0`)

But there's no way to compare two local workspace directories against each other.

### Use Case

A user has two local copies of workspaces:
```bash
~/project-a/pixi.toml  # pulled data-science:v1, then modified
~/project-b/pixi.toml  # pulled data-science:v2
```

They want to see the semantic diff (package additions, removals, version changes) between
these two local copies — without hitting the registry.

### When This Comes Up

1. Comparing modifications across two experiment directories
2. Comparing a global workspace copy vs a local project copy
3. Offline comparison with no network access
4. Quick "what's different between these two project dirs?" check

## Design

### Path Detection Heuristic

Arguments that "look like paths" are treated as local paths. An argument is path-like if:
- It starts with `/` (absolute path)
- It starts with `./` or `../` (explicit relative path)
- It starts with `~` (home directory)
- It is exactly `.` (current directory)

Everything else is treated as a workspace reference (`workspace:tag`).

This is unambiguous because workspace names cannot start with `/`, `./`, `../`, or `~`.
Users who want to diff a subdirectory named `foo` write `./foo` (same convention as git).

### Supported Combinations

With path detection, all combinations of path and ref arguments work:

| Command | Source | Target |
|---------|--------|--------|
| `nebi diff ./a ./b` | local path | local path |
| `nebi diff . ~/project-b` | current dir | local path |
| `nebi diff ~/a data-science:v2.0` | local path | registry ref |
| `nebi diff data-science:v1.0 ./b` | registry ref | local path |
| `nebi diff data-science:v1.0 data-science:v2.0` | registry ref | registry ref |

### Semantic Diff Engine

Path-based diffs reuse the existing `diff.CompareToml()` and `diff.CompareLock()` functions.
These operate on `[]byte` content, so they work identically regardless of whether the bytes
came from the registry or from a local file read.

### Interaction with `-C`/`--path` Flag

The existing `-C`/`--path` flag sets the "context directory" — where to find the `.nebi`
metadata for origin-based diffs. It does NOT participate in path-based diffing.

When both positional path arguments are provided, `-C` is ignored (there's no `.nebi` to read).

### Output Labels

For path arguments, the diff header shows the resolved absolute path:

```
--- /home/user/project-a
+++ /home/user/project-b
@@ pixi.toml @@
 [dependencies]
-numpy = ">=2.0.0,<3"
+numpy = ">=2.4.1,<3"
+scipy = ">=1.17.0,<2"
```

### JSON Output

For `--json`, path-based refs use `"type": "local"` with the resolved path:

```json
{
  "source": {"type": "local", "path": "/home/user/project-a"},
  "target": {"type": "local", "path": "/home/user/project-b"},
  "pixi_toml": { ... }
}
```

### Error Handling

| Scenario | Behavior |
|----------|----------|
| Path doesn't exist | Error: "directory not found: /path/to/dir" |
| Path has no pixi.toml | Error: "/path/to/dir/pixi.toml: no such file" |
| Path has no pixi.lock | Warning only; lock diff skipped |

## Implementation

### Changes Required

1. **Add `isPathLike(arg string) bool`** helper in `cmd/nebi/diff.go`
2. **Modify `runDiff`** dispatch to detect path-like arguments
3. **Add `runDiffTwoPaths(path1, path2 string)`** for path-vs-path comparison
4. **Modify `runDiffRefVsLocal`** to also handle path-vs-ref mixed cases
5. **Add `readLocalWorkspace(dir string) (toml, lock []byte, err error)`** helper to DRY up local file reading
6. **Update command help text** to document path syntax
7. **Add tests** for path detection and path-based diff dispatch

### Complexity

Low — approximately 80 lines of new code plus tests. The diff engine needs zero changes.
