# Issue: `nebi serve` Visual Separation in Help Output

**Status: Implemented**

## Problem

`nebi serve` starts a long-running server process. Every other command is a client-side
CLI operation. In the original help output, `serve` appeared in a flat alphabetical list
alongside `pull`, `push`, `shell`, etc. This made it hard for users to understand that
`serve` is fundamentally different.

Original `Available Commands` section:

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

## Solution: Cobra Command Groups

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
// ...

// Assign server commands
serveCmd.GroupID = "server"
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

## Alternatives Considered

| Approach | Why Not |
|----------|---------|
| Gray ANSI text for `serve` | Terminal background color issues (gray on dark = invisible), accessibility concerns, cobra doesn't support per-command styling without custom template |
| Separate binary (`nebi-server`) | Overkill for one command, complicates build/install/distribution |
| Custom help template | Reinvents what cobra already provides natively |
| Hide `serve` from default help | Discoverability loss, confuses users who self-host |

## Precedent

- **Docker**: "Management Commands" vs "Commands" (uses cobra groups)
- **kubectl**: "Basic Commands", "Deploy Commands", "Cluster Management Commands", etc.
- **gh** (GitHub CLI): Groups commands by category

## References

- [Cobra AddGroup API](https://pkg.go.dev/github.com/spf13/cobra)
- [Cobra command groups commit](https://github.com/spf13/cobra/commit/2169adb5749372c64cdd303864ae8a444da6350f)
