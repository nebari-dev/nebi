# Issue: PS1 Prompt Should Show Nebi Workspace:Tag

**Status: Future work**

## Problem

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

## The Name Mismatch

| Source | Value | Example |
|--------|-------|---------|
| Pixi (`pixi.toml [workspace] name`) | Project name | `data-science` |
| Nebi (server workspace + tag) | Workspace:tag | `data-science:v1.0` |

These can diverge. Someone could push a pixi project named `ml-tools` as nebi workspace
`team-analytics`. The prompt should reflect the nebi identity since that's what the user
asked for with `nebi shell team-analytics:v2`.

## How Pixi Sets PS1

From `pixi shell-hook` output:

```bash
export PIXI_PROMPT='(data-science) '
export PS1="(data-science) ${PS1:-}"
```

Key pixi flags:
- `--change-ps1=false` on `pixi shell` or `pixi shell-hook` — disables PS1 modification
- `PIXI_PROMPT` env var is set by pixi but **cannot** be pre-set to override (pixi's
  own value wins — confirmed by testing)

## Proposed Solution: `pixi shell-hook` Instead of `pixi shell`

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

## Design Considerations

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

## Implementation Complexity

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

## References

- [Pixi environment variables](https://pixi.prefix.dev/latest/reference/environment_variables/)
- [Pixi PS1 prompt modification PR](https://github.com/prefix-dev/pixi/pull/4101)
- [Pixi configuration: change-ps1](https://pixi.prefix.dev/latest/reference/pixi_configuration/)
