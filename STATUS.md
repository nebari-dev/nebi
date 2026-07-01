# Nebi Feature Status

Legend: 🟢 implemented &nbsp; 🟡 partial &nbsp; 🔴 missing

---

## 1. Solo developer

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want Nebi to work out of the box without requiring a server, database, or any additional infrastructure so I can start immediately. | 🟢 | |
| 2 | I want to initialize a new workspace so I can start tracking its metadata. | 🟢 | |
| 3 | I want to tag a workspace version so I can mark known-good states and return to them. | 🟡 | Content-addressed tags auto-created on push (`sha-<hash>`). User-defined tags exist server-side and can be listed (`nebi workspace tags`), but there is no `nebi tag` CLI command for explicit tagging. |
| 4 | I want to diff two workspace versions so I can understand what dependencies changed. | 🟢 | |
| 5 | I want to publish a workspace to an OCI source so I can pull it onto another machine. | 🟡 | `nebi publish --local` pushes directly to OCI. `nebi import` pulls from OCI. But there is no unified "source" concept — push and pull go through separate commands (`publish` / `import`) rather than a single `push`/`pull` that works against any configured source. |
| 6 | I want to pull a workspace's spec files from any connected source onto a new machine so I can install and recreate the environment there. | 🟡 | `nebi pull` pulls from a server; `nebi import` pulls from OCI. No unified command that works against any connected source. |
| 7 | I want to list all my local workspaces so I can see what I have and where they are located. | 🟢 | |
| 8 | I want to delete a workspace I no longer need. | 🟢 | |
| 9 | I want to manage my workspaces through a graphical interface so I can browse, create, and version them without using the terminal. | 🟡 | Wails desktop app scaffolding exists (`main.go`, `app.go`) with embedded frontend. Not yet built or packaged as a distributable app. |
| 10 | When a workspace fails to install, I want a clear explanation of what went wrong and assurance that my previous working environment is still intact. | 🟢 | |
| 11 | I want confirmation before destructive actions like deleting a workspace and feedback when async operations like pulling or installing complete. | 🟡 | Overwrite confirmations exist on `pull` and `import`. Registry removal is confirmed. Not universally applied (e.g. no confirmation on `workspace remove`). |
| 12 | I want `nebi install` to materialize a workspace's environment from its spec files so I don't need to invoke pixi directly. | 🔴 | No `nebi install` command exists. `nebi shell` and `nebi run` exist but shell out to pixi directly. |
| 13 | I want to add an OCI registry as a source so I can browse and pull workspaces without needing a Nebi server. | 🟡 | `nebi registry add --local` stores registry config locally. `nebi import` pulls from OCI. But registries are not integrated as browseable "sources" in the CLI or UI — there is no `nebi source` command or unified source list. |
| 14 | I want to browse a registry source, see available repositories as workspaces, and inspect their spec files before pulling. | 🔴 | `ListRepositories` and `ListTags` exist in `internal/oci/browser.go` but are not exposed through any CLI command or UI. |

## 2. Team developer

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to log in to a Nebi server through my organization's browser-based identity provider so I can connect to the server. | 🟢 | |
| 2 | In a managed environment like JupyterHub, I want Nebi to authenticate using a pre-existing bearer token from my session so I don't have to log in again. | 🟡 | `NEBI_AUTH_TOKEN` env var supports token-based auth, but no automatic session detection from JupyterHub or similar managed environments. |
| 3 | I want my available workspaces to be visible as soon as I'm connected to a source so I don't need to know exact names ahead of time. | 🟡 | Works for server connections. OCI registries are not integrated as browseable sources — no registry repository listing in CLI or UI. |
| 4 | I want to pull a workspace's spec files from a source so I can inspect them or install the environment when I'm ready. | 🟡 | `nebi pull` pulls spec files from server (and auto-installs). `nebi import` pulls from OCI. No inspect-before-install workflow — pulling always installs. |
| 5 | In a managed environment, I want Nebi-managed workspaces to appear as Jupyter kernel options so I can select them without leaving my notebook. | 🔴 | No Jupyter kernel integration exists. |
| 6 | Using the CLI, I want to run a command to check for newer workspace versions of my pulled workspaces so I can decide when to update. | 🔴 | No update-check command exists. |
| 7 | Using the UI, I want updates for my pulled workspaces to surface automatically through background polling with a manual refresh button so I can see what's new at a glance. | 🟡 | Frontend polls workspace data (2s interval), but there is no dedicated update-available notification or diff surfacing UI. |
| 8 | I want to know where my environments are stored on disk so I can point my tools at them. | 🟢 | |
| 9 | I want to disconnect from a Nebi server so I can remove it from my local configuration. | 🟢 | |
| 10 | I want to connect to and switch between multiple sources including servers and registries so I can access workspaces from different origins. | 🔴 | Only a single server URL is stored at a time. No multi-source concept exists. |
| 11 | When a workspace fails to install, I want a clear explanation of what went wrong and assurance that my previous working environment is still intact. | 🟢 | |
| 12 | I want confirmation before destructive actions like deleting a workspace and feedback when async operations like pulling or installing complete. | 🟡 | Same as solo developer #11. |
| 13 | I want to connect the desktop app to my team's Nebi server so I can browse, inspect, and pull workspaces through a graphical interface without using the CLI. | 🟢 | Remote proxy endpoints in the local-mode server bridge the desktop app to a remote server. Connect/disconnect flow, workspace browsing, and admin pages all work through the proxy. |
| 14 | I want all my workspaces — from all connected sources including servers and registries — to appear in a single list with a clear source indicator. | 🔴 | No multi-source concept exists. Workspace list only shows server workspaces. |
| 15 | I want the same editing capabilities for remote workspaces in the desktop app as I have for local workspaces — editing the manifest, installing packages, publishing versions — assuming write permission on the server. | 🟡 | RemoteWorkspaceDetail page and PixiTomlEditor exist. "Save & Install" button triggers server-side install. Full parity with local workspace editing is unclear. |
| 16 | In the UI, I want to install a workspace's environment by clicking a button so I can materialize it on my machine — distinct from browsing spec files, which is automatic and free. | 🔴 | No install-to-local-machine button exists in the UI. "Save & Install" saves to server, not local machine. |
| 17 | I want to see the install status of each workspace in the unified list (not installed, installed v12, update available) so I know which environments are ready to use at a glance. | 🔴 | No per-workspace install status in the list UI. |
| 18 | Using the CLI, I want `nebi install` to materialize a pulled workspace's environment so I don't need to invoke pixi directly. | 🔴 | Same as solo developer #12. |
| 19 | I want to add an OCI registry as a source alongside Nebi servers, so workspaces from all sources appear in one unified list. | 🔴 | Same as solo developer #13 — no multi-source concept. |
| 20 | Browsing a registry source, I want to see available repositories as read-only workspaces — I can browse spec files and tags, but cannot edit or push. | 🔴 | `ListRepositories` exists in OCI package but not exposed in CLI or UI. |

## 3. Environment manager

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to create a new workspace so I can publish it for my organization. | 🟢 | |
| 2 | I want to edit an existing workspace so I can add, remove, or update packages and publish a new version. | 🟡 | Users edit pixi.toml locally and push/publish a new version; there is no in-UI manifest editor that directly modifies server-side workspaces. |
| 3 | I want to publish a new workspace version so my team can pull the latest changes. | 🟢 | |
| 4 | I want to publish workspaces from both the CLI and the UI so I can choose the workflow that fits my context. | 🟢 | |
| 5 | I want each workspace version to have a clear, unambiguous version lineage so I can track its history. | 🟢 | |
| 6 | I want to share a workspace with specific users and groups at read-only, edit, or admin levels so I control who can view, modify, or manage each workspace. | 🟢 | |
| 7 | I want to remove sharing access from a user or group on a workspace so I can revoke their permissions when needed. | 🟢 | |
| 8 | I want to deprecate a workspace version so team developers see a warning when they check for updates, signaling that they should migrate. | 🔴 | No deprecation feature exists. |
| 9 | I want to archive a workspace version so it can no longer be pulled, while leaving existing local copies untouched. | 🔴 | No archive feature exists. |
| 10 | I want to publish a workspace to an OCI source so anyone with that registry connected can discover and pull it. | 🟡 | `nebi publish --local` pushes to OCI; server-side `nebi publish --registry <name>` pushes via server. Registry-connected discovery not implemented (no multi-source). |
| 11 | I want a workspace I publish to an OCI source to appear as a read-only workspace for anyone who has that registry connected. | 🔴 | No multi-source concept; no way for registry-connected users to see OCI-published workspaces in their workspace list. |
| 12 | I want to see the status and outcome of async operations like workspace pushes and builds so I know when they're done and can diagnose failures. | 🟢 | Job queue, worker, and the jobs API endpoint all exist. The UI surfaces job status. |

## 4. Server administrator

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to deploy the Nebi server as a binary, container, or Helm chart so I can run it in my organization's infrastructure. | 🟡 | Binary build exists. Dockerfile and docker-compose images (`docker/pixi.Dockerfile`, `docker/uv.Dockerfile`) exist. Helm chart in `chart/` exists. No pre-built container images are published. |
| 2 | I want to configure the Nebi server through a config file or environment variables so it fits into my existing infrastructure. | 🟢 | |
| 3 | I want to configure a production setup with OIDC authentication, Postgres as the database, Valkey as the job queue, Docker as the execution backend, and an enforced environment storage path. | 🟡 | OIDC, Postgres, Valkey, and enforced storage path all supported. Docker executor is **not implemented** — only the local executor exists. |
| 4 | I want the server to expose health checks, metrics, tracing, and structured logging so I can monitor it in production. | 🟡 | Health check (`/health`) and structured logging (slog, configurable JSON format) exist. No Prometheus metrics endpoint or OpenTelemetry tracing. |
| 5 | I want to lock down the list of available Nebi servers on Nebi clients so I control where workspaces are pulled from and users cannot add, remove, or modify them. | 🔴 | No server policy or client lockdown mechanism exists. |
| 6 | I want to disable standalone operation on Nebi clients so all workspace usage goes through the server. | 🔴 | No disable-standalone flag or policy exists. |
| 7 | I want to upgrade the Nebi server to a new version with confidence that existing workspaces, workspace versions, and connected clients will continue to function without data loss or manual intervention. | 🟡 | GORM auto-migration runs on startup, but there is no versioned migration tooling or documented upgrade procedure. |
| 8 | I want Nebi to derive user identities, group memberships, and admin status from the OIDC ID token claims so I don't need to maintain a separate user directory or manage group membership manually within Nebi. | 🟡 | OIDC proxy flow (`findOrCreateProxyUser`, `SyncOIDCGroups`, `syncRolesFromGroups`) already populates users, groups, and admin from claims on every request. Native user/group CRUD (admin UI, CreateUser, CreateGroup, membership management) still exists as a parallel path and should be removed. |

## 5. Automation / machine consumer

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to authenticate to a Nebi server using machine-to-machine OIDC credentials so I can pull workspaces without human interaction. | 🔴 | No M2M / client-credentials OIDC flow exists. `NEBI_AUTH_TOKEN` env var can carry a JWT but obtaining one without human interaction is not addressed. |
| 2 | I want to pull a specific workspace version so my builds are deterministic. | 🟡 | `nebi pull <workspace>:<tag>` supports versioned pulls. No `nebi pull --version N` using version numbers. |
| 3 | I want the Nebi client to operate in a headless mode with structured output, no color, and clean exit codes so I can integrate it into scripts and pipelines. | 🟡 | Many commands support `--json`. Exit codes are used. No explicit `--no-color` or headless toggle flag exists. |
| 4 | I want to pull a workspace from an OCI source so I can recreate environments without Nebi server access or credentials. | 🟡 | `nebi import` pulls from OCI without server access. No unified `nebi pull` that works against any source type. |
| 5 | I want `nebi pull` to work against any configured source — server or registry — using the same command and output format. | 🔴 | `nebi pull` only works with a server. `nebi import` is a separate command for OCI. |

## 6. Distributed compute operator

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to configure worker nodes to authenticate to a Nebi server using machine-to-machine OIDC credentials. | 🔴 | Same gap as automation #1. |
| 2 | I want worker nodes to pull a specific workspace version so the computing environment is identical across all nodes. | 🟡 | Same as automation #2. |
| 3 | I want worker nodes to automatically pull a workspace and execute within its environment so I don't need to manage Docker images or manual synchronization. | 🔴 | No Dask/Ray/Slurm or cluster orchestration integration exists. |
| 4 | I want worker nodes to pull workspaces from an OCI source so I can scale out without all nodes needing direct access to the Nebi server. | 🟡 | `nebi import` supports OCI pull without server access. |

## 7. Security & compliance officer

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want an audit trail of who pulled which workspace version and when so I can demonstrate compliance during reviews. | 🟡 | Audit log infrastructure exists (DB table, API endpoint, admin UI page). Covers group sync, workspace import, and workspace publishing. Pull events and permission changes are not yet recorded. |
| 2 | I want Nebi to expose the resolved package list per workspace version so I can run my own CVE scanner against it and surface the results. | 🔴 | No resolved package list API exists. |
| 3 | I want a per-workspace-version license report listing all packages and their licenses so I can identify problematic dependencies. | 🔴 | No license report exists. |
| 4 | I want to export a workspace version's package list in a standard SBOM format so I can feed it into compliance tools. | 🔴 | No SBOM export exists. |

---

## Existing Features Not Captured in Stories

These features are currently implemented but do not map to any user story. Each should be discussed: does it need a story, or can it be removed?

| Feature | Description | Suggestion |
|---|---|---|
| **Basic auth** | Username/password authentication (`NEBI_AUTH_TYPE=basic`) used as default. | Stories only mention OIDC. Decide whether basic auth is a deliberate server-admin feature (add story) or legacy to remove. |
| **UV package manager adapter** | Config key `package_manager.default_type: uv` and `NEBI_PACKAGE_MANAGER_UV_PATH` exist. Factory is registered but no UV implementation is wired up. | Dead code. Remove unless a story drives UV support. |
| **`nebi shell`** | Activate a pixi shell in a workspace by name. | Partially overlaps with solo #12 / team #18 (`nebi install`). Shell is interactive; install is a one-shot materialization. Consider adding a story for interactive shell access. |
| **`nebi run`** | Run a pixi task in a workspace by name. | Same as `nebi shell` — distinct from `nebi install`. Add a story for running tasks in workspaces. |
| **`nebi completion`** | Shell completion generation for bash/zsh/fish/powershell. | Low priority; add a story for CLI polish or leave as uncaptured DX. |
| **`nebi info`** | System info dump (server connection, auth status, config paths). | Partially overlaps with other stories; add a story for troubleshooting/debugging. |
| **`nebi status`** | Show workspace sync status (modified files, server drift). | Add a story for workspace status overview. |
| **`nebi workspace prune`** | Remove tracking entries for workspaces whose on-disk directories no longer exist. | Add a story for workspace cleanup. |
| **`nebi workspace tags`** | List tags for a remote workspace. | Complement to solo dev story #3 (tagging). |
| **Content-addressed tags** | Auto-created `sha-<hash>` tags on every push/publish for deduplication. | Not user-visible as a story; belongs in implementation design. |
| **Server-side environment builds** | The server currently runs `pixi install` (full download + build) during create, solve, rollback, add, and remove operations. This consumes gigabytes of disk per workspace to produce (a) a package list and (b) a disk-size measurement — neither of which any story requires. | Planned simplification: use `pixi add --no-install` / `pixi remove --no-install` / `pixi install --no-install` to resolve and update the lock file without downloading packages. Parse `pixi.lock` for package metadata instead of `pixi list`. Stop measuring disk size. Rollback restores toml+lock from the version snapshot without re-running pixi. **Open question:** resolution speed must be confirmed before removing the async job pipeline — if SAT solving is fast enough (seconds), every operation becomes synchronous and the queue/worker/logstream/SSE stack can be dropped. |
| **Job queue (Valkey)** | The job queue currently supports a Valkey backend for distributed job processing across multiple server instances. | After removing `pixi install`, no operation downloads packages — the heaviest remaining work is SAT solving (seconds). Even if that proves too slow for synchronous HTTP and the job system is retained, a distributed queue is unnecessary: an in-process goroutine (buffered channel) handles sub-minute async work without the operational complexity of an external Valkey dependency. **Open question:** confirm resolution speed; if async is still needed, downgrade Valkey to a goroutine. |
| **Group management UI/API** | Full admin CRUD for groups (list, create, get, update name/description, hard-delete with casbin cleanup), membership management (add/remove members, list members), OIDC group sync (`SyncOIDCGroups` reconciles `groups` claim on every request, creating OIDC-sourced groups and membership), group admin promotion (`/admin/groups/:id/grant-admin` adds casbin `g` rule), group registry access grants, and group workspace sharing (`/workspaces/:id/share-group`). Groups carry a `source` column distinguishing `"native"` (Nebi-managed) from `"oidc"` (read-only, IdP-managed). Frontend: Groups admin page, CreateGroupDialog, GroupMembersDialog, group picker in ShareDialog. | Now covered by server admin story #8, which says Nebi should derive groups from the IdP, not maintain its own directory. The native group CRUD, membership management, and admin promotion UI/API should be removed. |
| **User management UI/API** | Full admin CRUD for users (list with `is_admin` flag, create with bcrypt password, get by ID, toggle admin, delete), user auto-provisioning from OIDC claims (`findOrCreateProxyUser` on every request), admin role sync from OIDC group membership (`syncRolesFromGroups` checks against `PROXY_ADMIN_GROUPS`). Users have `PasswordHash` (empty for OIDC-provisioned users). Frontend: UserManagement admin page, CreateUserDialog, user picker in ShareDialog. | Now covered by server admin story #8, which says Nebi should derive users from the IdP. The CreateUser dialog (requires password), ToggleAdmin button, and DeleteUser action should be removed. The user list endpoint should be accessible to non-admins for sharing autocomplete. |
| **Registry management UI/API** | Admin page for managing OCI registry configurations on the server. | Add a server admin story for registry configuration. |
| **Branding/theme configuration** | Server-served CSS custom properties for UI branding. | Add a server admin story for white-labeling/branding. |
