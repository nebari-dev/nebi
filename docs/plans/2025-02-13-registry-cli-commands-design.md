# Registry CLI Commands Design

## Overview

Add `registry create` and `registry delete` commands to the nebi CLI, enabling users to manage OCI registries without using the web UI.

## Background

Currently, the CLI only supports `nebi registry list`. Creating and deleting registries requires the web UI at `/admin/registries`. This adds CLI support for the full lifecycle.

We intentionally omit `registry update` - users can delete and recreate if they need to change settings. This keeps the CLI simple and avoids awkward partial-update flag semantics.

## Commands

### `nebi registry create`

```
nebi registry create --name <name> --url <url> [--username <user>] [--password-stdin] [--default]
```

| Flag | Required | Description |
|------|----------|-------------|
| `--name` | Yes | Display name for the registry (e.g., "ghcr") |
| `--url` | Yes | Registry URL (e.g., "ghcr.io", "quay.io/myorg") |
| `--username` | No | Username for authentication |
| `--password-stdin` | No | Read password from stdin |
| `--default` | No | Set as the default registry |

**Behavior:**
- If `--username` provided without `--password-stdin`, prompt interactively for password
- If `--password-stdin` provided, read password from stdin (for scripting)
- If registry with same name already exists, error out
- On success, print confirmation to stderr

**Examples:**
```bash
# Interactive - prompts for password
nebi registry create --name ghcr --url ghcr.io --username myuser

# Programmatic - read password from stdin
echo "$TOKEN" | nebi registry create --name ghcr --url ghcr.io --username myuser --password-stdin

# Public registry (no auth needed)
nebi registry create --name dockerhub --url docker.io --default
```

### `nebi registry delete`

```
nebi registry delete <name> [--force]
```

| Argument/Flag | Required | Description |
|---------------|----------|-------------|
| `<name>` | Yes | Name of the registry to delete |
| `--force`, `-f` | No | Skip confirmation prompt |

**Behavior:**
- Look up registry by name (not ID)
- If not found, error out
- Unless `--force`, prompt: "Delete registry 'name'? [y/N]"
- On success, print confirmation to stderr

**Examples:**
```bash
# Interactive - prompts for confirmation
nebi registry delete ghcr

# Scripted - skip confirmation
nebi registry delete ghcr --force
```

## Design Decisions

### No `--password` flag
Passwords passed as flags appear in shell history and process lists. Instead:
- Interactive: prompt using `term.ReadPassword()` (like `nebi login`)
- Scripted: use `--password-stdin` (like `docker login`)

### No API token support
The API token field is only used for browsing private Quay.io repositories in the web UI. The CLI doesn't have browse functionality, so we skip it. Users who need this can use the web UI.

### No `registry update` command
Update requires awkward partial-update semantics (which fields are changing vs keeping?). Delete + create is clearer and handles the rare cases where credentials need rotation.

### Delete by name only
UUIDs are clunky for CLI use. Names are human-readable and what users will remember.

### Confirmation on delete
Matches the web UI behavior, which shows a confirmation dialog. Use `--force` for scripting.

## Implementation Notes

### Files to modify
- `cmd/nebi/registry.go` - Add `registryCreateCmd` and `registryDeleteCmd`
- `internal/cliclient/types.go` - Already has `CreateRegistryRequest` (may need to add `DefaultRepository` field)

### Existing patterns to follow
- `cmd/nebi/login.go` - Password prompting with `term.ReadPassword()`
- `cmd/nebi/publish.go` - Registry resolution pattern in `resolveRegistryID()`
- `internal/cliclient/registries.go` - Already has `CreateRegistry()` and `DeleteRegistry()` methods

### Error cases
- Registry name already exists (create)
- Registry name not found (delete)
- Server connection failure
- Authentication failure
