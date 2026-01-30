# Nebi CLI Demo Workflow

A step-by-step walkthrough to exercise the core nebi CLI commands.
Assumes you have a running nebi server and have built the CLI (`go build -o nebi ./cmd/nebi`).

---

## 1. Login

```bash
# Login to a nebi server
nebi login http://localhost:8080

# Verify you're logged in (will show server URL)
nebi version
```

## 2. Registry Setup

```bash
# Add an OCI registry for pushing workspaces
nebi registry add myregistry oci://localhost:5000 --default

# List configured registries
nebi registry list
```

## 3. Create a Workspace Locally

```bash
# Initialize a new pixi workspace
pixi init demo && cd demo

# Add dependencies
pixi add "python>=3.11" "numpy>=1.24"
```

## 4. Push to Server

```bash
# Dry-run first to see what would be pushed
nebi push demo-workspace:v1.0 --dry-run

# Push for real
nebi push demo-workspace:v1.0
```

## 5. Browse Workspaces on Server

```bash
# List all workspaces on the server
nebi workspace list

# Show tags for our workspace
nebi workspace tags demo-workspace

# Show detailed info
nebi workspace info demo-workspace
```

## 6. Pull a Workspace

```bash
# Pull into a new directory
cd /tmp
nebi pull demo-workspace:v1.0 -o pulled-workspace

# Or pull into global storage
nebi pull demo-workspace:v1.0 --global
```

## 7. Check Status (Drift Detection)

```bash
cd /tmp/pulled-workspace

# Status should be "clean" right after pull
nebi status

# Now modify pixi.toml
echo '[dependencies]
scipy = ">=1.11"' >> pixi.toml

# Status now shows "modified"
nebi status

# Verbose mode shows per-file status
nebi status -v

# JSON output for scripting
nebi status --json
```

## 8. View Diffs

```bash
# Show what changed locally vs what was pulled (origin)
nebi diff

# Show only pixi.toml changes
nebi diff --toml

# Compare against current remote tag
nebi diff --remote

# Show lock file package-level changes
nebi diff --lock

# JSON output
nebi diff --json
```

### Comparing Two Remote References

```bash
# Push a v2.0 with the modified workspace
nebi push demo-workspace:v2.0

# Compare two versions on the server
nebi diff demo-workspace:v1.0 demo-workspace:v2.0

# Compare a remote ref against your local workspace
nebi diff demo-workspace:v1.0
```

## 9. Local Workspace Management

```bash
# List all locally-pulled workspaces with drift indicators
nebi workspace list --local
# Output shows: WORKSPACE, TAG, STATUS (clean/modified/missing/stale), LOCATION

# If any workspace directories have been deleted, prune them
nebi workspace prune
```

### Status Meanings

| Status     | Meaning                                      |
|------------|----------------------------------------------|
| `clean`    | No changes since pull                        |
| `modified` | pixi.toml or pixi.lock has been edited       |
| `missing`  | Tracked files deleted (directory still exists)|
| `stale`    | Directory no longer exists (pruneable)       |
| `unknown`  | Cannot determine (no .nebi metadata)         |

## 10. Shell and Run

`nebi shell` and `nebi run` wrap `pixi shell` and `pixi run` with workspace
lookup and auto-initialization. All arguments pass through directly to pixi.
The `--manifest-path` flag is not supported; use `pixi shell`/`pixi run` directly if needed.

```bash
# Shell into current directory (auto-initializes if not tracked)
nebi shell

# Shell into a global workspace by name
nebi shell my-datascience

# All args pass through to pixi shell
nebi shell my-datascience -e default

# Run a pixi task in the current directory (auto-initializes)
nebi run my-task

# Run a task in a global workspace
nebi run my-datascience my-task

# Run in a local directory
nebi run ./some-project my-task

# Args pass through to pixi run
nebi run -e dev my-task
```

## 11. Push with Drift Awareness

```bash
cd /tmp/pulled-workspace

# Modify something
echo 'pandas = ">=2.0"' >> pixi.toml

# Push shows drift warnings (modified files noted)
nebi push demo-workspace:v2.1

# Pushing the same tag warns about overwriting
nebi push demo-workspace:v1.0
```

## 12. Cleanup

```bash
# Delete workspace from server
nebi workspace delete demo-workspace

# Remove local registry
nebi registry remove myregistry

# Logout
nebi logout
```

---

## Exit Codes

| Code | Meaning                        |
|------|--------------------------------|
| 0    | Success / no changes           |
| 1    | Changes detected (status/diff) |
| 2    | Error                          |

## Useful Flags

| Flag         | Commands      | Description                      |
|--------------|---------------|----------------------------------|
| `--json`     | status, diff  | Machine-readable JSON output     |
| `--remote`   | status, diff  | Check against current remote tag |
| `--lock`     | diff          | Show lock file package details   |
| `--toml`     | diff          | Show only pixi.toml changes      |
| `-C`/`--path`| status, diff  | Specify workspace directory      |
| `--dry-run`  | push          | Preview without pushing          |
| `--global`   | pull          | Pull to central storage          |
| `--force`    | pull          | Overwrite existing files         |
| `--local`    | workspace list| Show locally-pulled workspaces   |
