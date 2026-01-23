# Issue: `nebi serve` Visual Separation & PS1 Prompt Customization

Two related UX issues with the CLI help output and shell experience.

## Problem 1: `nebi serve` Mixed In With Client Commands

`nebi serve` starts a long-running server process. Every other command is a client-side
CLI operation. In the current help output, `serve` appears in a flat alphabetical list
alongside `pull`, `push`, `shell`, etc. This makes it hard for users to understand that
`serve` is fundamentally different.

Current `Available Commands` section:

```
Available Commands:
  completion  Generate the autocompletion script for the specified shell
  diff        Show workspace differences
  help        Help about any command
  login       Login to Nebi server
  logout      Logout from server
  pull        Pull workspace from server
  push        Push workspace to registry
  registry    Manage OCI registries
  serve       Start the Nebi server       <-- buried here
  shell       Activate workspace shell
  status      Show workspace drift status
  version     Print the version
  workspace   Manage workspaces
```

### Solution: Cobra Command Groups

Cobra v1.10.2 (already in go.mod) has a built-in `AddGroup`/`GroupID` API for exactly
this purpose. You define named groups on the parent command, then assign each subcommand
to a group. Cobra renders separate sections in help output automatically.

```go
// In init() in main.go:
rootCmd.AddGroup(
    &cobra.Group{ID: "client", Title: "Client Commands:"},
    &cobra.Group{ID: "server", Title: "Server Commands:"},
)

// Assign client commands
loginCmd.GroupID = "client"
logoutCmd.GroupID = "client"
registryCmd.GroupID = "client"
workspaceCmd.GroupID = "client"
pushCmd.GroupID = "client"
pullCmd.GroupID = "client"
shellCmd.GroupID = "client"
statusCmd.GroupID = "client"
diffCmd.GroupID = "client"

// Assign server commands
serveCmd.GroupID = "server"

// Optionally group built-in commands
rootCmd.SetHelpCommandGroupID("") // leaves help/completion ungrouped (renders under "Additional Commands:")
rootCmd.SetCompletionCommandGroupID("")
```

Result:

```
Client Commands:
  diff        Show workspace differences
  login       Login to Nebi server
  logout      Logout from server
  pull        Pull workspace from server
  push        Push workspace to registry
  registry    Manage OCI registries
  shell       Activate workspace shell
  status      Show workspace drift status
  workspace   Manage workspaces

Server Commands:
  serve       Start the Nebi server

Additional Commands:
  completion  Generate the autocompletion script
  help        Help about any command
  version     Print the version
```

### Alternatives Considered

| Approach | Why Not |
|----------|---------|
| Gray ANSI text for `serve` | Terminal background color issues (gray on dark = invisible), accessibility concerns, cobra doesn't support per-command styling without custom template |
| Separate binary (`nebi-server`) | Overkill for one command, complicates build/install/distribution |
| Custom help template | Reinvents what cobra already provides natively |
| Hide `serve` from default help | Discoverability loss, confuses users who self-host |

### Precedent

- **Docker**: "Management Commands" vs "Commands" (uses cobra groups)
- **kubectl**: "Basic Commands", "Deploy Commands", "Cluster Management Commands", etc.
- **gh** (GitHub CLI): Groups commands by category

### Implementation

~15 lines added to `main.go` init(). The `Long` description on `rootCmd` can then be
simplified since the grouping makes the manual "Server commands:" / "Client commands:"
listing in the description redundant.

---

## Problem 2: PS1 Prompt Should Show Nebi Workspace:Tag

Users want the shell prompt to show which nebi workspace and tag they're in:

```
(data-science:v1.0) [balast@nirvana my-pixi-env]$
```

Currently, `nebi shell` delegates entirely to `pixi shell`, which sets:

```bash
export PIXI_PROMPT='(data-science) '
export PS1="(data-science) ${PS1:-}"
```

The prompt shows the **pixi project name** (from `pixi.toml` `[workspace] name`), not
the nebi workspace identity.

### The Name Mismatch

| Source | Value | Example |
|--------|-------|---------|
| Pixi (`pixi.toml [workspace] name`) | Project name | `data-science` |
| Nebi (server workspace + tag) | Workspace:tag | `data-science:v1.0` |

These can diverge. Someone could push a pixi project named `ml-tools` as nebi workspace
`team-analytics`. The prompt should reflect the nebi identity since that's what the user
asked for with `nebi shell team-analytics:v2`.

### How Pixi Sets PS1

From `pixi shell-hook` output:

```bash
export PIXI_PROMPT='(data-science) '
export PS1="(data-science) ${PS1:-}"
```

Key pixi flags:
- `--change-ps1=false` on `pixi shell` or `pixi shell-hook` — disables PS1 modification
- `PIXI_PROMPT` env var is set by pixi but **cannot** be pre-set to override (pixi's
  own value wins — confirmed by testing)

### Proposed Solution: `pixi shell-hook` Instead of `pixi shell`

Instead of running `pixi shell` (which takes over PS1), nebi would:

1. Run `pixi shell-hook --change-ps1=false` to get the activation script
2. Start a new interactive shell with pixi activation sourced + custom PS1

**For bash:**

```bash
bash --rcfile <(
  # Source user's normal bashrc
  [ -f ~/.bashrc ] && source ~/.bashrc
  # Activate pixi environment without PS1 changes
  eval "$(pixi shell-hook --change-ps1=false --manifest-path /path/to/pixi.toml)"
  # Set nebi-aware prompt
  export NEBI_WORKSPACE="data-science"
  export NEBI_TAG="v1.0"
  export NEBI_IN_SHELL=1
  export PS1="(${NEBI_WORKSPACE}:${NEBI_TAG}) ${PS1}"
)
```

**For zsh:** Similar approach using `ZDOTDIR` to a temp directory with a custom `.zshrc`.

### Design Considerations

1. **Tag display**: Should the tag always be shown? Maybe omit for `latest`:
   - `(data-science:v1.0)` — explicit tag
   - `(data-science)` — latest/untagged

2. **Pixi environment name**: If not `default`, append it:
   - `(data-science:v1.0|dev)` — non-default pixi environment

3. **Environment variables**: Useful even without PS1 customization:
   - `NEBI_WORKSPACE` — workspace name
   - `NEBI_TAG` — tag
   - `NEBI_IN_SHELL=1` — detect nebi shell context (like `PIXI_IN_SHELL`)

4. **Drift indicator in prompt** (future): Could show `*` when workspace is modified:
   - `(data-science:v1.0*) $` — workspace has local modifications

5. **User override**: Respect a `NEBI_PS1_FORMAT` env var or config option for users
   who want to customize the format.

### Implementation Complexity

Medium — requires rewriting `execPixiShell` in `cmd/nebi/pixi.go` to:
- Read `.nebi` metadata for workspace/tag info
- Detect user's shell (`$SHELL`)
- Generate shell-specific init script
- Handle edge cases (no `.nebi` file, unknown shell, nested nebi shells)

### Current Code (`cmd/nebi/pixi.go:51`)

```go
func execPixiShell(dir string, envName string) {
    pixiPath := mustPixiBinary()
    args := []string{"shell"}
    if envName != "" {
        args = append(args, "-e", envName)
    }
    cmd := exec.Command(pixiPath, args...)
    cmd.Dir = dir
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil { ... }
}
```

This would change to get the shell-hook output, compose an init script, and exec into
the user's shell with that init script.

---

## Priority

1. **Serve separation** — small change, immediate UX improvement. Do first.
2. **PS1 prompt** — medium effort, worth designing carefully. Future phase.

## References

- [Cobra AddGroup API](https://pkg.go.dev/github.com/spf13/cobra)
- [Cobra command groups commit](https://github.com/spf13/cobra/commit/2169adb5749372c64cdd303864ae8a444da6350f)
- [Pixi environment variables](https://pixi.prefix.dev/latest/reference/environment_variables/)
- [Pixi PS1 prompt modification PR](https://github.com/prefix-dev/pixi/pull/4101)
- [Pixi configuration: change-ps1](https://pixi.prefix.dev/latest/reference/pixi_configuration/)
