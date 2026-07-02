# Nebi Feature Status

Legend: 🟢 implemented &nbsp; 🟡 partial &nbsp; 🔴 missing

---

## 1. Developer

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to create, list, and delete local workspaces so I can manage my workspace lifecycle. | 🟢 | |
| 2 | I want to tag workspace versions and diff them so I can mark known-good states and understand what changed between versions. | 🟡 | Content-addressed tags auto-created on push (`sha-<hash>`). User-defined tags exist server-side and can be listed (`nebi workspace tags`), but there is no `nebi tag` CLI command for explicit tagging. Diff works. |
| 3 | I want to add, list, switch between, and remove sources so I can access workspaces from multiple origins in one place. | 🔴 | Only a single server URL is stored at a time. No multi-source concept exists. OCI registries can be added locally but are not integrated as browseable sources — no `nebi source` command or unified source list. |
| 4 | I want a unified workspace list across all connected sources that shows each workspace's source, install status, and whether it's read-only, so I can discover and browse environments without knowing where they live. | 🔴 | Works for server connections. OCI registries are not integrated as browseable sources — no registry repository listing in CLI or UI. No per-workspace install status in the list. No multi-source workspace list exists. |
| 5 | I want Nebi to work out of the box without requiring a server, database, or any additional infrastructure so I can start immediately. | 🟢 | |
| 6 | I want to publish a workspace to an OCI source so I can pull it onto another machine. | 🟡 | `nebi publish --local` pushes directly to OCI. `nebi import` pulls from OCI. No unified source concept — push and pull go through separate commands (`publish` / `import`). |
| 7 | I want to pull a workspace's spec files from any connected source and install its environment — via CLI or a button in the UI — with feedback on progress and where the environment lives on disk. `nebi install` implies a pull when needed. | 🔴 | `nebi pull` fetches spec files from server; `nebi import` fetches from OCI. Neither materializes the environment — no `nebi install` command exists. No install-to-local-machine button in the UI. Disk location is visible. |
| 8 | I want to manage workspaces through a graphical interface — browsing, editing, installing, and publishing — with the same capabilities for both local and remote workspaces, so the desktop app is a full management interface regardless of where the workspace lives. | 🟡 | Wails desktop app scaffolding exists (`main.go`, `app.go`) with embedded frontend. RemoteWorkspaceDetail page and PixiTomlEditor exist. Not yet built or packaged as a distributable app. Full parity with local workspace editing unclear. |
| 9 | I want clear explanations when operations fail — with assurance my previous environment is intact — and confirmation before destructive actions, so I can trust Nebi not to lose my work. | 🟡 | Overwrite confirmations exist on `pull` and `import`. Registry removal is confirmed. Not universally applied (e.g. no confirmation on `workspace remove`). |
| 10 | I want to log in to a Nebi server through my organization's browser-based identity provider so I can connect to the server. | 🟢 | |
| 11 | In a managed environment like JupyterHub, I want Nebi to authenticate using a pre-existing bearer token from my session so I don't have to log in again. | 🟡 | `NEBI_AUTH_TOKEN` env var supports token-based auth, but no automatic session detection from JupyterHub or similar managed environments. |
| 12 | In a managed environment, I want Nebi-managed workspaces to appear as Jupyter kernel options so I can select them without leaving my notebook. | 🔴 | No Jupyter kernel integration exists. |
| 13 | I want to check for newer versions of my pulled workspaces — via CLI command or automatic background polling in the UI — so I can decide when to update. | 🔴 | Frontend polls workspace data (2s interval), but there is no dedicated update-available notification or CLI update-check command. |

## 2. Environment manager

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to create, edit, and publish workspace versions — from both CLI and UI — so my team can pull the latest changes. | 🟢 | `nebi push` publishes; UI has PublishDialog. Create and edit supported in both CLI and UI (PixiTomlEditor with Save & Install for remote workspaces). |
| 2 | I want each workspace version to have clear lineage, with the ability to deprecate versions (warning on update check) and archive them (block pulls), so I can guide my team toward the right versions. | 🔴 | Version lineage exists. No deprecation feature. No archive feature. |
| 3 | I want to grant and revoke workspace access for users and groups at read-only, edit, or admin levels, so I control who can view, modify, or manage each workspace. | 🟢 | |
| 4 | I want to publish a workspace to an OCI source so anyone with that registry connected can discover and pull it as a read-only workspace, making the registry a distribution channel alongside the Nebi server. | 🔴 | `nebi publish --local` pushes to OCI; server-side `nebi publish --registry <name>` pushes via server. No multi-source concept — registry-connected users cannot see OCI-published workspaces in their workspace list. |

## 3. Server administrator

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to deploy the Nebi server — as a binary, container, or Helm chart — and configure it through a config file or environment variables, with production options for OIDC, Postgres, Docker execution, and enforced storage paths. | 🟡 | Binary build exists. Dockerfile and docker-compose images exist. Helm chart exists. No pre-built container images. OIDC, Postgres, and enforced storage path supported. Docker executor is **not implemented** — only local executor exists. |
| 2 | I want the server to expose health checks, metrics, tracing, and structured logging so I can monitor it in production. | 🟡 | Health check (`/health`) and structured logging (slog, configurable JSON format) exist. No Prometheus metrics endpoint or OpenTelemetry tracing. |
| 3 | I want to lock down client behavior — restricting available Nebi servers and disabling standalone operation — so I control where workspaces come from. | 🔴 | No server policy or client lockdown mechanism exists. No disable-standalone flag. |
| 4 | I want to upgrade the Nebi server to a new version with confidence that existing workspaces, workspace versions, and connected clients will continue to function without data loss or manual intervention. | 🟡 | GORM auto-migration runs on startup, but there is no versioned migration tooling or documented upgrade procedure. |
| 5 | I want Nebi to derive user identities, group memberships, and admin status from the OIDC ID token claims so I don't need to maintain a separate user directory or manage group membership manually within Nebi. | 🟡 | OIDC proxy flow (`findOrCreateProxyUser`, `SyncOIDCGroups`, `syncRolesFromGroups`) populates users, groups, and admin from claims on every request. Native user/group CRUD (admin UI, CreateUser, CreateGroup, membership management) still exists as a parallel path and should be removed. |

## 4. Automation / machine consumer

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to authenticate to a Nebi server using machine-to-machine OIDC credentials so I can pull workspaces without human interaction. | 🔴 | No M2M / client-credentials OIDC flow exists. `NEBI_AUTH_TOKEN` env var can carry a JWT but obtaining one without human interaction is not addressed. |
| 2 | I want the Nebi client to operate in a headless mode with structured output, no color, and clean exit codes so I can integrate it into scripts and pipelines. | 🟡 | Many commands support `--json`. Exit codes are used. No explicit `--no-color` or headless toggle flag exists. |
| 3 | I want to pull a specific workspace version from any configured source with the same command and output format, so my scripts are deterministic and source-agnostic. | 🔴 | `nebi pull <workspace>:<tag>` supports versioned pulls from server. `nebi import` pulls from OCI. No unified pull command across source types. No `nebi pull --version N` using version numbers. |

## 5. Distributed compute operator

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to configure worker nodes to authenticate to a Nebi server using machine-to-machine OIDC credentials. | 🔴 | Same gap as automation #1. |
| 2 | I want worker nodes to automatically pull a workspace and execute within its environment so I don't need to manage Docker images or manual synchronization. | 🔴 | No Dask/Ray/Slurm or cluster orchestration integration exists. |
| 3 | I want worker nodes to pull a specific workspace version from any source so environments are identical across all nodes. | 🟡 | `nebi pull` from server; `nebi import` from OCI. No unified command across source types. |

## 6. Security & compliance officer

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want audit trails, package lists for CVE scanning, license reports, and SBOM exports per workspace version, so I can demonstrate compliance. | 🔴 | Audit log infrastructure exists (DB table, API endpoint, admin UI page) but covers limited events — pull events and permission changes are not recorded. No resolved package list API, license report, or SBOM export. |

---

## Existing Features Not Captured in Stories

These features are currently implemented but do not map to any user story. Each should be discussed: does it need a story, or can it be removed?

| Feature | Description | Suggestion |
|---|---|---|
| **Basic auth** | Username/password authentication (`NEBI_AUTH_TYPE=basic`) used as default. | Stories only mention OIDC. Decide whether basic auth is a deliberate server-admin feature (add story) or legacy to remove. |
| **UV package manager adapter** | Config key `package_manager.default_type: uv` and `NEBI_PACKAGE_MANAGER_UV_PATH` exist. Factory is registered but no UV implementation is wired up. | Dead code. Remove unless a story drives UV support. |
| **`nebi shell`** | Activate a pixi shell in a workspace by name. | Partially overlaps with developer story #7 (`nebi install`). Shell is interactive; install is a one-shot materialization. Consider adding a story for interactive shell access. |
| **`nebi run`** | Run a pixi task in a workspace by name. | Same as `nebi shell` — distinct from `nebi install`. Add a story for running tasks in workspaces. |
| **`nebi completion`** | Shell completion generation for bash/zsh/fish/powershell. | Low priority; add a story for CLI polish or leave as uncaptured DX. |
| **`nebi info`** | System info dump (server connection, auth status, config paths). | Partially overlaps with other stories; add a story for troubleshooting/debugging. |
| **`nebi status`** | Show workspace sync status (modified files, server drift). | Add a story for workspace status overview. |
| **`nebi workspace prune`** | Remove tracking entries for workspaces whose on-disk directories no longer exist. | Add a story for workspace cleanup. |
| **`nebi workspace tags`** | List tags for a remote workspace. | Complement to developer story #2 (tagging). |
| **Content-addressed tags** | Auto-created `sha-<hash>` tags on every push/publish for deduplication. | Not user-visible as a story; belongs in implementation design. |
| **Async operation status (UI/API)** | Job queue, worker, and the jobs API endpoint surface status of workspace pushes and builds in the UI. | No story currently captures this. Consider adding one for environment manager or folding into story #1. |
| **Server-side environment builds** | The server currently runs `pixi install` (full download + build) during create, solve, rollback, add, and remove operations. This consumes gigabytes of disk per workspace to produce (a) a package list and (b) a disk-size measurement — neither of which any story requires. | Planned simplification: use `pixi add --no-install` / `pixi remove --no-install` / `pixi install --no-install` to resolve and update the lock file without downloading packages. Parse `pixi.lock` for package metadata instead of `pixi list`. Stop measuring disk size. Rollback restores toml+lock from the version snapshot without re-running pixi. **Open question:** resolution speed must be confirmed before removing the async job pipeline — if SAT solving is fast enough (seconds), every operation becomes synchronous and the queue/worker/logstream/SSE stack can be dropped. |
| **Job queue (Valkey)** | The job queue currently supports a Valkey backend for distributed job processing across multiple server instances. | After removing `pixi install`, no operation downloads packages — the heaviest remaining work is SAT solving (seconds). Even if that proves too slow for synchronous HTTP and the job system is retained, a distributed queue is unnecessary: an in-process goroutine (buffered channel) handles sub-minute async work without the operational complexity of an external Valkey dependency. **Open question:** confirm resolution speed; if async is still needed, downgrade Valkey to a goroutine. |
| **Group management UI/API** | Full admin CRUD for groups (list, create, get, update name/description, hard-delete with casbin cleanup), membership management (add/remove members, list members), OIDC group sync (`SyncOIDCGroups` reconciles `groups` claim on every request, creating OIDC-sourced groups and membership), group admin promotion (`/admin/groups/:id/grant-admin` adds casbin `g` rule), group registry access grants, and group workspace sharing (`/workspaces/:id/share-group`). Groups carry a `source` column distinguishing `"native"` (Nebi-managed) from `"oidc"` (read-only, IdP-managed). Frontend: Groups admin page, CreateGroupDialog, GroupMembersDialog, group picker in ShareDialog. | Now covered by server admin story #5, which says Nebi should derive groups from the IdP, not maintain its own directory. The native group CRUD, membership management, and admin promotion UI/API should be removed. |
| **User management UI/API** | Full admin CRUD for users (list with `is_admin` flag, create with bcrypt password, get by ID, toggle admin, delete), user auto-provisioning from OIDC claims (`findOrCreateProxyUser` on every request), admin role sync from OIDC group membership (`syncRolesFromGroups` checks against `PROXY_ADMIN_GROUPS`). Users have `PasswordHash` (empty for OIDC-provisioned users). Frontend: UserManagement admin page, CreateUserDialog, user picker in ShareDialog. | Now covered by server admin story #5, which says Nebi should derive users from the IdP. The CreateUser dialog (requires password), ToggleAdmin button, and DeleteUser action should be removed. The user list endpoint should be accessible to non-admins for sharing autocomplete. |
| **Registry management UI/API** | Admin page for managing OCI registry configurations on the server. | Add a server admin story for registry configuration. |
| **Branding/theme configuration** | Server-served CSS custom properties for UI branding. | Add a server admin story for white-labeling/branding. |
