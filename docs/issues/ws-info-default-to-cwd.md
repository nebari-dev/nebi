# Issue: `nebi ws info` Should Default to Current Directory

## Problem

`nebi ws info` currently requires a `<workspace>` argument. When run from a directory
that contains a `.nebi` metadata file (i.e., a pulled workspace), it should default to
showing info for that workspace without requiring the user to type the name.

## Current Behavior

```bash
$ cd ~/my-project   # has .nebi file from a prior pull
$ nebi ws info
# Error: accepts 1 arg(s), received 0
```

## Analysis

### Current Implementation

**`nebi ws info <workspace>`** (`workspace.go`):
- Requires exactly 1 argument (`cobra.ExactArgs(1)`)
- Takes a workspace **name** (server-side lookup)
- Queries the **server API** for workspace details (ID, status, package manager, owner, packages)
- This is a **server-side info command**

**`nebi status`** (`status.go`):
- Takes **no** arguments (`cobra.NoArgs`)
- Operates on the **current directory** (or `-C` path)
- Reads the `.nebi` metadata file to detect local drift
- This is a **local state command**

### Key Difference

Despite the design doc (`diff-workflow.md`) stating "`nebi status` = `nebi workspace info`
(with enhanced output)", the implementations serve different needs:

| | `nebi ws info <name>` | `nebi status` |
|---|---|---|
| **Input** | Workspace name (server lookup) | Current directory (`.nebi` file) |
| **Data source** | Server API | Local `.nebi` + file hashes |
| **Purpose** | "What is this workspace on the server?" | "Has my local copy drifted?" |
| **Network required** | Always | Only with `--remote` |

### Precedent: `nebi shell`

`nebi shell` already implements the "detect workspace from cwd" pattern:
1. Check if `.nebi` exists in current directory
2. Fallback: check for `pixi.toml`
3. If neither: error with helpful message

### Should `nebi ws info` and `nebi status` Be Aliases?

**No.** They serve different needs:
- `nebi status` = quick, offline, compact drift check (like `git status`)
- `nebi ws info` = detailed, server-querying info (like `gh repo view`)

The right relationship: `nebi ws info` (no args) is a **superset** -- it shows local drift
status AND server-side details.

## Solution: Option C (Combined Local + Server Info)

When `nebi ws info` is run without arguments:
1. Read `.nebi` file from current directory (or `-C` path) to get workspace name
2. Show **local status** (drift detection, same as `nebi status`)
3. Show **server info** (same as today's `nebi ws info <name>`)
4. Combined output gives a complete picture of "this workspace"

When run WITH an argument, behavior is unchanged (server-only lookup).

### Detection Logic

```
1. absDir = filepath.Abs(path flag, or ".")
2. nf, err = nebifile.Read(absDir)
3. if err -> show error with hints
4. workspaceName = nf.Origin.Workspace
5. -> proceed with local drift check + server lookup
```

### Error Messages

When no `.nebi` file exists:
```
Error: Not a nebi workspace directory (no .nebi file found)

Hint: Specify a workspace name: nebi workspace info <name>
      Or pull a workspace first: nebi pull <workspace>:<tag>
```

### Output Format (No Args, From Workspace Directory)

```
$ cd ~/my-project && nebi ws info

Local:
  Workspace: data-science:v1.0
  Registry:  ds-team
  Pulled:    2025-01-15 10:30:00 (7 days ago)
  Digest:    sha256:abc123...
  Status:    modified
    pixi.toml:  modified
    pixi.lock:  clean

Server:
  Name:            data-science
  ID:              abc-123
  Status:          active
  Package Manager: pixi
  Owner:           jsmith
  Size:            48023 bytes
  Created:         2025-01-10
  Updated:         2025-01-18

  Packages (5):
    - numpy (>=2.0)
    - pandas (>=2.0)
    ...
```
