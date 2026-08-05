# Sandboxed Environment Builds

**Date:** 2026-08-05
**Issue:** https://github.com/nebari-dev/nebi/issues/445
**Status:** Approved design, pending implementation

## Problem

Untrusted environment builds (`pixi install`, `pixi lock`, and the other pixi
subcommands that evaluate caller-supplied manifests) run as ordinary child
processes of the nebi server or worker. Two consequences, verified against
main:

1. **Credential exposure.** No `exec.Cmd` in `internal/executor` or
   `internal/pkgmgr` ever sets `cmd.Env`, so every pixi subprocess inherits the
   full server environment: `NEBI_DATABASE_DSN` (with password),
   `NEBI_AUTH_JWT_SECRET`, `NEBI_QUEUE_VALKEY_ADDR`, and any `${VAR}`-expanded
   registry credentials from config-managed registries.
2. **No tenant separation on disk.** All workspaces live flat under
   `storage.workspaces_dir` with uniform permissions and a single uid. A
   malicious build backend in workspace A can read or modify workspace B's
   `pixi.toml`, `pixi.lock`, and installed environment.

Issue 445's acceptance criteria for this layer: malicious build code cannot
read the application database password and cannot read or modify another
workspace's files.

## Decisions made during brainstorming

- **Scope:** both credential scrubbing and filesystem isolation in one PR.
- **Mechanism:** Landlock LSM self-sandboxing via a re-exec helper. Rejected:
  bubblewrap (needs unprivileged user namespaces, blocked by default
  Docker/K8s seccomp profiles, fights the nebi-pack chart hardening) and
  per-job uid switching (needs CAP_SETUID/SETGID/CHOWN; nebi-pack drops ALL).
- **Posture:** `strict` by default in team mode (builds fail if Landlock is
  unavailable), `off` by default in local/desktop mode. Env scrubbing applies
  unconditionally in all modes on all platforms.

## Architecture

Two cooperating pieces inside the existing nebi binary. No new images, no
Kubernetes dependency, no changes to the worker dispatch loop, queue, or job
model.

### `internal/sandbox` package

The single construction point for pixi subprocess commands.

- `sandbox.Command(ctx, cfg, spec)` returns an `*exec.Cmd`.
  - Always sets `cmd.Env` to the scrubbed allowlist (below).
  - When sandboxing is active, the argv becomes
    `<nebi-binary> sandbox-exec --mode=<m> --allow-rw=<p> [--allow-ro=<p>...]
    [--allow-ro-file=<p>...] [--allow-port=<n>...] -- <real argv>`.
  - When mode is `off` (or the platform is not Linux and mode is not
    `strict`), the real argv runs directly, still with the scrubbed env.
- `spec` carries: the workspace path (RW root), the working directory, the
  real argv, and the job-scoped `HOME`/`TMPDIR`.

### `nebi sandbox-exec` hidden subcommand

A minimal re-exec shim, Linux-only:

1. Parse `--allow-rw`, `--allow-ro`, `--allow-ro-file`, `--allow-port`,
   `--mode`.
2. Apply the Landlock ruleset with `github.com/landlock-lsm/go-landlock`
   (pure Go when `CGO_ENABLED=0`; the library handles thread synchronisation
   itself, so the caller does not lock the OS thread):
   - Filesystem: RW dirs from `--allow-rw`, RO dirs from `--allow-ro`, RO
     files from `--allow-ro-file`, RW files for `/dev/null`, `/dev/urandom`,
     `/dev/random`, `/dev/zero`.
   - This is attempted at ABI v2 (kernel 5.19+) first so the `refer` right
     can be granted on the workspace, falling back to ABI v1 (kernel 5.13+).
     Landlock always denies reparenting a file across directories unless
     `refer` is granted, and v1 cannot grant it, so under v1 a build cannot
     rename a file from one workspace subdirectory into another. Package
     managers do exactly that when they stage a download and then move it
     into their cache. v1 remains the hard floor for confinement.
   - TCP connect restriction (ABI v4, kernel 6.7+): connect allowed only to
     `--allow-port` values; bind denied. Best-effort: absence of ABI v4 is a
     logged warning, not a failure, even in strict mode.
3. `syscall.Exec` the real command. execve preserves the Landlock domain, so
   pixi and every process it spawns stays confined.
4. Failure semantics: any sandbox-setup failure exits with reserved code
   **125** and a diagnostic on stderr. In `permissive` mode, FS-restriction
   unavailability degrades to a warning and the command runs unconfined.

### What a confined build can access

- **RW:** its own workspace directory (`{baseDir}/{name}-{uuid}`), which also
  contains the per-job `tmp/` and, because `HOME` points inside the
  workspace, the pixi/rattler package cache. Per-workspace cache is the safe
  default; a shared cache would let one tenant poison packages for another.
  The perf trade-off (no cross-workspace cache reuse) is accepted and
  documented.
- **RO directories:** `/usr`, `/lib`, `/lib64`, `/bin`, `/sbin`, `/opt`, the
  TLS trust stores (`/etc/ssl`, `/etc/pki`, `/etc/ca-certificates`), and the
  directory containing the pixi binary.
- **RO files:** `/etc/resolv.conf`, `/etc/hosts`, `/etc/nsswitch.conf`,
  `/etc/localtime`, `/etc/passwd`, `/etc/group`.
- Paths that do not exist on a given host are skipped.

  `/etc` is deliberately **not** granted wholesale. `internal/config/config.go`
  searches `/etc/nebi/` for `config.yaml`, and both `database.dsn` and
  `auth.jwt_secret` are file-loadable fields, so a readable `/etc` would hand
  build code the very credentials the environment allowlist removes. Only the
  specific trust-store directories and resolver files above are granted.
- **Network:** TCP connect only to `sandbox.allowed_ports` (default
  `[80, 443]`). The database (5432) and Valkey (6379) become unreachable from
  build code even though they share the pod network. Kernels below 6.7 skip
  this with a warning.
- **Environment allowlist:** `PATH`, job-scoped `HOME` and `TMPDIR`, `LANG`,
  `LC_ALL`, `SSL_CERT_FILE`, `SSL_CERT_DIR`, `HTTP_PROXY`, `HTTPS_PROXY`,
  `NO_PROXY`. Everything else is dropped, notably `NEBI_*` and any registry
  credential variables.

## Configuration

New config block (with viper `BindEnv` entries, following the existing
pattern in `internal/config/config.go`):

```yaml
sandbox:
  # strict | permissive | off
  # Default: strict when mode=team, off when mode=local
  mode: strict
  # TCP ports build code may connect to (Landlock ABI v4+ kernels only)
  allowed_ports: [80, 443]
  # Wall-clock ceiling per build job
  build_timeout: 30m
```

Env vars: `NEBI_SANDBOX_MODE`, `NEBI_SANDBOX_ALLOWED_PORTS`,
`NEBI_SANDBOX_BUILD_TIMEOUT`.

Mode resolution happens at config load: an explicit value wins; otherwise
team mode defaults to `strict` and local mode to `off`.

## Integration points

All pixi exec sites route through `sandbox.Command`:

- `internal/executor/local.go:170` (`runPixiLock`) and `:317`
  (`InstallEnvironment`, the primary untrusted-code hot spot).
- The six exec sites in `internal/pkgmgr/pixi/pixi.go` (init, add, remove,
  update, lock, list — lines 135, 210, 285, 347, 414, 500 on main).
Two exec sites are deliberately left alone: the `pixi --version` probes in
`internal/pkgmgr/pixi/pixi.go:68` and `internal/pkgmgr/pixi/installer.go:252`.
They run first-party pixi code with a fixed argument and evaluate no user
input, so confining them buys nothing and risks breaking pixi's own startup
requirements.

The worker applies `sandbox.build_timeout` via `context.WithTimeout` around
`executeJob`. Exit code 125 from the shim is mapped to a distinct
"sandbox setup failed" job error so operators can tell infrastructure
problems from build failures.

## Error handling

| Situation | strict | permissive | off |
|---|---|---|---|
| Landlock FS available | confined | confined | unconfined, scrubbed env |
| Landlock FS unavailable | job fails fast with actionable error naming `NEBI_SANDBOX_MODE` | warning in job log + server log, runs unconfined | n/a |
| Landlock net (ABI v4) unavailable | warning, FS-only confinement | warning, FS-only confinement | n/a |
| Non-Linux host | job fails (team servers are Linux; macOS desktop is local mode = off) | warning, runs unconfined | n/a |

## Testing

- **Unit** (all platforms): env allowlist construction; shim argv building;
  config mode resolution (team defaults strict, local defaults off, explicit
  values win); exit-code-125 mapping in the worker.
- **Integration** (Linux only, `t.Skip` when Landlock ABI v1 is absent;
  CI runs ubuntu-24.04, kernel 6.8, which has ABI v4): the acceptance test
  issue 445 asks for. A deliberately malicious "build" must fail to
  (a) read a planted fake DSN from the parent environment,
  (b) read a sibling workspace's `pixi.toml`,
  (c) write outside its workspace root,
  (d) TCP-connect to a local listener standing in for the database;
  while a legitimate build in its own workspace succeeds.
- Existing executor/pkgmgr tests keep passing: tests run in local mode where
  the sandbox defaults to off, and env scrubbing preserves `PATH`.

## Non-goals (explicit follow-ups)

- Kubernetes Job-per-build executor (infra-level isolation; needs client-go,
  nebi-pack chart work, queue ack/retry, log shipping).
- Queue visibility-timeout/ack semantics (jobs lost if a worker dies
  mid-build are a pre-existing gap).
- Moving API-side workspace writes (`SavePixiToml`, `PushVersion`, import
  staging) behind the executor. These are trusted code paths; the sandbox
  targets untrusted build execution.
- Killing the whole process group on build timeout. `exec.CommandContext`
  signals only the direct child, so pixi's own grandchildren can outlive a
  timeout. Fixing it needs `Setpgid`, which is not portable to the Windows
  desktop build, so it needs its own platform-split change. The escaped
  processes remain Landlock-confined; this is a resource-cleanup gap, not a
  containment gap.
- Per-workspace volumes / RWX PVC split in nebi-pack.
- Registry or private-channel credentials for builds (would be per-job
  credential files, never env inheritance; needs its own design).

## Acceptance mapping to issue 445

| Criterion | Met by |
|---|---|
| Build code cannot read the DB password | env allowlist (all modes) + Landlock net deny of 5432 (defense in depth) |
| Build code cannot read/modify another workspace's files | Landlock FS confinement to the job's workspace |
| Security tests use a deliberately malicious build and verify containment | the Linux integration test above |
| No K8s API token in package runner | already handled chart-side (nebi-pack PR 45) |
| Job pods deleted / per-job infra isolation | follow-up (K8s Job executor) |
