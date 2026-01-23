# Shell Activation Disambiguation

## Problem

When a user runs `nebi shell data-science:v1.0` and multiple copies of that workspace:tag exist (global and/or multiple local pulls), there is no way to specify which one to activate. The current implementation silently picks the most recently pulled copy when multiple local copies exist, which is surprising and non-deterministic from the user's perspective.

## Current Behavior

- Global copy always wins over local copies (good, keep this)
- If multiple local copies exist, silently picks the most recent by `PulledAt` — no user choice
- No flags exist to override resolution (only `--env` / `-e` for pixi environment)

## Proposed Solution

### New Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--global` | `-g` | Force use of global copy |
| `--local` | `-l` | Force use of local copies (skip global); if multiple, triggers interactive selection |
| `--path` | `-C` | Use workspace at a specific directory path |

### Resolution Priority

```
1. If -C/--path specified  → use that exact directory (error if not a nebi workspace)
2. If --global specified   → use global copy (error if none exists)
3. If --local specified    → filter to local copies only, then:
                              - If one: use it
                              - If multiple: interactive selection (or error if no TTY)
4. Default (no flags):
     a. If global exists   → use global (global always wins)
     b. If one local       → use it
     c. If multiple local  → interactive selection
     d. If none            → pull from server (store globally)
```

### Interactive Selection UX

When multiple local copies exist and disambiguation is needed:

```
$ nebi shell data-science:v1.0

Multiple local copies found for data-science:v1.0:
  [1] ~/project-a  (pulled 2 days ago, modified)
  [2] ~/project-b  (pulled 1 hour ago, clean)

Select [1-2] or use -C to specify:
```

Information shown per entry:
- Path (shortened with `~`)
- Relative pull time
- Drift status: `clean` / `modified`

### Non-Interactive Mode (no TTY)

When stdin is not a TTY and disambiguation is needed, print an actionable error and exit with code 2:

```
Error: Multiple local copies of data-science:v1.0 found, cannot disambiguate without a TTY.

Available copies:
  ~/project-a  (pulled 2 days ago)
  ~/project-b  (pulled 1 hour ago)

Use -C to specify:
  nebi shell data-science:v1.0 -C ~/project-a
```

### Flag Conflicts

- `--global` and `--local` are mutually exclusive → error
- `-C`/`--path` with either `--global` or `--local` → error (path is already explicit)

## Files to Modify

- `cmd/nebi/shell.go` — add flags, update resolution logic, add interactive prompt
- `cmd/nebi/shell_test.go` — add tests for new flags and disambiguation behavior

## Related

- [docs/design/duplicate-pulls.md](../design/duplicate-pulls.md) — Shell Activation section
- [docs/design/diff-workflow.md](../design/diff-workflow.md) — Shell Activation Behavior section
