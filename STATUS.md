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
| 5 | I want to push a workspace to an OCI registry so I can pull it onto another machine. | 🟢 | |
| 6 | I want to pull a workspace from an OCI registry onto a new machine so I can recreate the environment there. | 🟢 | |
| 7 | I want to list all my local workspaces so I can see what I have and where they are located. | 🟢 | |
| 8 | I want to delete a workspace I no longer need. | 🟢 | |
| 9 | I want to manage my workspaces through a graphical interface so I can browse, create, and version them without using the terminal. | 🟡 | Wails desktop app scaffolding exists (`main.go`, `app.go`) with embedded frontend. Not yet built or packaged as a distributable app. |
| 10 | When a workspace fails to install, I want a clear explanation of what went wrong and assurance that my previous working environment is still intact. | 🟢 | |
| 11 | I want confirmation before destructive actions like deleting a workspace and feedback when async operations like pulling or installing complete. | 🟡 | Overwrite confirmations exist on `pull` and `import`. Registry removal is confirmed. Not universally applied (e.g. no confirmation on `workspace remove`). |

## 2. Team developer

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to log in to a Nebi server through my organization's browser-based identity provider so I can connect to the server. | 🟢 | |
| 2 | In a managed environment like JupyterHub, I want Nebi to authenticate using a pre-existing bearer token from my session so I don't have to log in again. | 🟡 | `NEBI_AUTH_TOKEN` env var supports token-based auth, but no automatic session detection from JupyterHub or similar managed environments. |
| 3 | I want my available workspaces to be visible as soon as I'm connected to a Nebi server so I don't need to know exact names ahead of time. | 🟢 | |
| 4 | I want to pull a workspace from the server so I can recreate the environment and start working. | 🟢 | |
| 5 | In a managed environment, I want Nebi-managed workspaces to appear as Jupyter kernel options so I can select them without leaving my notebook. | 🔴 | No Jupyter kernel integration exists. |
| 6 | Using the CLI, I want to run a command to check for newer workspace versions of my pulled workspaces so I can decide when to update. | 🔴 | No update-check command exists. |
| 7 | Using the UI, I want updates for my pulled workspaces to surface automatically through background polling with a manual refresh button so I can see what's new at a glance. | 🟡 | Frontend polls workspace data (2s interval), but there is no dedicated update-available notification or diff surfacing UI. |
| 8 | I want to know where my environments are stored on disk so I can point my tools at them. | 🟢 | |
| 9 | I want to disconnect from a Nebi server so I can remove it from my local configuration. | 🟢 | |
| 10 | I want to connect to and switch between multiple Nebi servers so I can access workspaces from different sources, such as a development server and a production server. | 🔴 | Only a single server URL is stored at a time. |
| 11 | When a workspace fails to install, I want a clear explanation of what went wrong and assurance that my previous working environment is still intact. | 🟢 | |
| 12 | I want confirmation before destructive actions like deleting a workspace and feedback when async operations like pulling or installing complete. | 🟡 | Same as solo developer #11. |

## 3. Environment manager

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to create a new workspace so I can publish it for my organization. | 🟢 | |
| 2 | I want to edit an existing workspace so I can add, remove, or update packages and publish a new version. | 🟡 | Workspace metadata editing needs clarification. Users edit pixi.toml locally and push/publish a new version; there is no in-UI manifest editor. |
| 3 | I want to publish a new workspace version so my team can pull the latest changes. | 🟢 | |
| 4 | I want to publish workspaces from both the CLI and the UI so I can choose the workflow that fits my context. | 🟢 | |
| 5 | I want each workspace version to have a clear, unambiguous version lineage so I can track its history. | 🟢 | |
| 6 | I want to share a workspace with specific users and groups at read-only, edit, or admin levels so I control who can view, modify, or manage each workspace. | 🟢 | |
| 7 | I want to remove sharing access from a user or group on a workspace so I can revoke their permissions when needed. | 🟢 | |
| 8 | I want to deprecate a workspace version so team developers see a warning when they check for updates, signaling that they should migrate. | 🔴 | No deprecation feature exists. |
| 9 | I want to archive a workspace version so it can no longer be pulled, while leaving existing local copies untouched. | 🔴 | No archive feature exists. |

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

## 5. Automation / machine consumer

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to authenticate to a Nebi server using machine-to-machine OIDC credentials so I can pull workspaces without human interaction. | 🔴 | No M2M / client-credentials OIDC flow exists. `NEBI_AUTH_TOKEN` env var can carry a JWT but obtaining one without human interaction is not addressed. |
| 2 | I want to pull a specific workspace version so my builds are deterministic. | 🟡 | `nebi pull <workspace>:<tag>` supports versioned pulls. No `nebi pull --version N` using version numbers. |
| 3 | I want the Nebi client to operate in a headless mode with structured output, no color, and clean exit codes so I can integrate it into scripts and pipelines. | 🟡 | Many commands support `--json`. Exit codes are used. No explicit `--no-color` or headless toggle flag exists. |

## 6. Distributed compute operator

| # | Story | Status | Comments |
|---|---|---|---|
| 1 | I want to configure worker nodes to authenticate to a Nebi server using machine-to-machine OIDC credentials. | 🔴 | Same gap as automation #1. |
| 2 | I want worker nodes to pull a specific workspace version so the computing environment is identical across all nodes. | 🟡 | Same as automation #2. |
| 3 | I want worker nodes to automatically pull a workspace and execute within its environment so I don't need to manage Docker images or manual synchronization. | 🔴 | No Dask/Ray/Slurm or cluster orchestration integration exists. |

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
| **`nebi push`** | Push workspace specs (pixi.toml + pixi.lock) to the Nebi server. Distinct from `publish` which pushes to OCI. | Add a story capturing server-side workspace upload (team dev or env manager). |
| **`nebi shell`** | Activate a pixi shell in a workspace by name. | Add a story for "run commands / open shell in a workspace". |
| **`nebi run`** | Run a pixi task in a workspace by name. | Same as `nebi shell`. |
| **`nebi completion`** | Shell completion generation for bash/zsh/fish/powershell. | Low priority; add a story for CLI polish or leave as uncaptured DX. |
| **`nebi info`** | System info dump (server connection, auth status, config paths). | Partially overlaps with other stories; add a story for troubleshooting/debugging. |
| **`nebi status`** | Show workspace sync status (modified files, server drift). | Add a story for workspace status overview. |
| **`nebi workspace prune`** | Remove tracking entries for workspaces whose on-disk directories no longer exist. | Add a story for workspace cleanup. |
| **`nebi workspace tags`** | List tags for a remote workspace. | Complement to solo dev story #3 (tagging). |
| **Content-addressed tags** | Auto-created `sha-<hash>` tags on every push/publish for deduplication. | Not user-visible as a story; belongs in implementation design. |
| **Remote proxy endpoints** | Local-mode server proxies browse requests to a remote Nebi server for hybrid local+remote workflows. | Could be a solo dev story for "browse remote workspaces from desktop app". |
| **Job system (queue + worker)** | Async job pipeline with memory or Valkey queue and local executor. | Infrastructure detail, not a user-facing story. Fine to leave uncaptured. |
| **Group management UI/API** | Admin CRUD for groups, membership management, OIDC group sync. | Underpins env manager #6/#7. Add an admin story if not fully covered. |
| **User management UI** | Admin page for listing, creating, and deleting users. | Add a server admin story for user lifecycle. |
| **Registry management UI/API** | Admin page for managing OCI registry configurations on the server. | Add a server admin story for registry configuration. |
| **Audit log UI** | Admin page for browsing audit log entries. | Maps to security officer #1. The story exists but wording should cover UI access. |
| **Branding/theme configuration** | Server-served CSS custom properties for UI branding. | Add a server admin story for white-labeling/branding. |
| **`nebi publish --local`** | Publish directly to an OCI registry without a server. | Covered by solo dev story #5. Remove from this list or keep as implementation detail. |
| **`nebi import`** | Pull a workspace bundle from an OCI registry without a server. | Covered by solo dev story #6. Remove from this list or keep as implementation detail. |
