# Proposal: Implicit Nebi tracking via pixi wrapper

## Problem

Currently, nebi requires explicit CLI commands (`nebi push`, `nebi pull`, `nebi status`, etc.) to manage environments. Users must remember to interact with nebi separately from their normal pixi workflow. This adds friction and makes it easy to forget to push changes or check status.

## Proposal

Create a wrapper that shadows the `pixi` command. When a user runs `pixi <subcommand>`, the wrapper:

1. Delegates to the real `pixi` binary for the actual operation
2. For certain subcommands, performs lightweight nebi bookkeeping afterward (e.g., drift detection)

This makes nebi tracking automatic — users just use `pixi` as normal and nebi awareness happens transparently.

### New command: `nebi init`

A new `nebi init` command registers the current workspace in the global `index.json`. This is the explicit entry point for tracking — once a workspace is initialized, the pixi wrapper can detect it and perform bookkeeping.

## How it works

### Wrapper mechanics

- The wrapper is installed with a higher PATH priority than the real `pixi` binary
- It locates the real `pixi` binary (e.g., via a config setting or by searching PATH beyond itself)
- It runs the real `pixi` command, passing through all arguments and stdio
- After the real command succeeds, it checks `index.json` to determine if this directory is a tracked nebi workspace, and performs bookkeeping accordingly

### Subcommand behavior

| pixi command | Nebi side effect |
|---|---|
| `pixi add <pkg>` | Detect drift — note that workspace has diverged from last known remote state |
| `pixi remove <pkg>` | Same as `pixi add` |
| `pixi lock` | Detect drift — re-locking can change pixi.lock |
| `pixi init` | No nebi side effect (use `nebi init` separately to start tracking) |
| `pixi install` | No side effect |
| `pixi shell` / `pixi run` | No side effect |
| Everything else | Pure pass-through |

For commands with side effects, the wrapper does NOT modify anything on the server. It only updates local tracking state (drift detection against stored layer digests in `index.json`).

### Index check flow (rough)

```
user runs: pixi add numpy

wrapper:
  1. run real `pixi add numpy` (pass-through)
  2. if pixi exits non-zero → exit with same code, do nothing
  3. look up cwd in index.json
  4. if tracked →
     a. recompute digests of pixi.toml + pixi.lock
     b. compare against stored layers
     c. if diverged, note status (e.g., print a one-liner like "nebi: modified since v1.0.0")
  5. if not tracked → do nothing
```

## Implementation

### Single Go binary with symlink-based dispatch

The nebi binary checks `os.Args[0]` at startup. If invoked as `pixi` (via symlink), it enters wrapper mode. If invoked as `nebi`, it runs the normal CLI. This is the same pattern used by BusyBox and ccache.

**Finding the real pixi**: The wrapper skips its own entry in PATH and searches for the next `pixi` binary. No config or stored path needed — if the user upgrades or moves pixi, it just works.

### `nebi setup` command

Rather than requiring users to manually create symlinks, a `nebi setup` command automates the process:

```bash
nebi setup
```

This would:
1. Create a symlink `~/.local/bin/pixi -> /path/to/nebi`
2. Verify that `~/.local/bin` appears in PATH before the real pixi location
3. Warn if PATH ordering is wrong and suggest how to fix it

`nebi setup --undo` would remove the symlink and restore direct pixi access.

## Open questions

- **Drift detection details**: When the wrapper detects divergence, what exactly does it do? Just print a message? Update a status field in `index.json`? The wrapper should not have any side effects on pixi.toml/pixi.lock — only on nebi's own tracking state.
- **Notification UX**: How verbose should the wrapper be? A single dimmed line on drift? Silent unless diverged? Configurable?
- **Pixi version compatibility**: The wrapper needs to stay compatible with future pixi versions. A good test suite against pixi's CLI behavior is needed.
- **`nebi init` semantics**: What information does `nebi init` capture? Just registers the cwd + pixi.toml digest in index.json? Or does it also associate with a remote server/spec?

## Notes

- Nebi tracks entire pixi workspaces (pixi.toml + pixi.lock), so multi-environment workspaces within a single pixi.toml are handled naturally.
- This is a potential redesign of the CLI, not an addition to the existing feature branch. No backwards compatibility constraints.

## Prior art

- `direnv` hooks into shell prompts to auto-load `.envrc`
- `nvm` / `pyenv` use shims in PATH to intercept commands
- `git` hooks (post-commit, etc.) for side effects after operations
