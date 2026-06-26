# Nebi User Stories

## Stakeholders

| # | Stakeholder | Summary |
|---|---|---|
| 1 | **Solo developer** | Individual developer operating standalone without a Nebi server. Wants versioned, reproducible environments. Desktop app possible here. |
| 2 | **Team developer** | Developer in a managed org. Pulls shared workspaces from a Nebi server. Needs discoverability, auth, fast pulls. |
| 3 | **Environment manager** | Curates which workspaces exist, defines manifests, publishes versions, sets access control and group membership. In larger orgs this is a dedicated role/team. |
| 4 | **Server administrator** | Deploys/configures the Nebi server, manages auth providers, DB, queue, Docker executor, encryption keys. |
| 5 | **Automation / machine consumer** | CI pipelines and other automated workflows. Needs M2M auth, headless client output, deterministic workspace version pinning. |
| 6 | **Distributed compute operator** | Operators of Dask, Ray, Slurm, or similar clusters who need identical environments across all nodes. |
| 7 | **Security & compliance officer** | Needs CVE scanning, SBOM, license tracking, audit trails for workspace usage across the org. |

---

## Glossary

**Workspace** — The manifest (pixi.toml), lockfile (pixi.lock), and optionally any bundled workspace files that together define a reproducible environment. This is what Nebi versions, shares, and publishes.

**Workspace version** — A specific, numbered instance of a workspace within its version lineage.

**Environment** — The materialized set of installed packages on disk, created by Pixi from a workspace.

**Nebi server** — The server component that stores workspaces, authenticates users, and serves workspaces to clients.

**Nebi client** — The CLI or desktop application that connects to a Nebi server or operates standalone. Its behavior can be restricted by server policy.

---

## 1. Solo developer

1. As a solo developer, I want Nebi to work out of the box without requiring a server, database, or any additional infrastructure so I can start immediately.
2. As a solo developer, I want to initialize a new workspace so I can start tracking its metadata.
3. As a solo developer, I want to tag a workspace version so I can mark known-good states and return to them.
4. As a solo developer, I want to diff two workspace versions so I can understand what dependencies changed.
5. As a solo developer, I want to push a workspace to an OCI registry so I can pull it onto another machine.
6. As a solo developer, I want to pull a workspace from an OCI registry onto a new machine so I can recreate the environment there.
7. As a solo developer, I want to list all my local workspaces so I can see what I have and where they are located.
8. As a solo developer, I want to manage my workspaces through a graphical interface so I can browse, create, and version them without using the terminal.

## 2. Team developer

1. As a team developer on my local machine, I want to log in to a Nebi server through my organization's browser-based identity provider so I can connect to the server.
2. As a team developer in a managed environment like JupyterHub, I want Nebi to authenticate using a pre-existing bearer token from my session so I don't have to log in again.
3. As a team developer, I want my available workspaces to be visible as soon as I'm connected to a Nebi server so I don't need to know exact names ahead of time.
4. As a team developer, I want to pull a workspace from the server so I can recreate the environment and start working.
5. As a team developer using the CLI, I want to run a command to check for newer workspace versions of my pulled workspaces so I can decide when to update.
6. As a team developer using the UI, I want updates for my pulled workspaces to surface automatically through background polling with a manual refresh button so I can see what's new at a glance.
7. As a team developer, I want to know where my environments are stored on disk so I can point my tools at them.
8. As a team developer, I want to disconnect from a Nebi server so I can remove it from my local configuration.
9. As a team developer, I want to connect to and switch between multiple Nebi servers so I can access workspaces from different sources, such as a development server and a production server.

## 3. Environment manager

1. As an environment manager, I want to create a new workspace so I can publish it for my organization.
2. As an environment manager, I want to publish a new workspace version so my team can pull the latest changes.
3. As an environment manager, I want to publish workspaces from both the CLI and the UI so I can choose the workflow that fits my context.
4. As an environment manager, I want each workspace version to have a clear, unambiguous version lineage so I can track its history.
5. As an environment manager, I want to share a workspace with specific users and groups at read-only, edit, or admin levels so I control who can view, modify, or manage each workspace.
6. As an environment manager, I want to remove sharing access from a user or group on a workspace so I can revoke their permissions when needed.
7. As an environment manager, I want to deprecate a workspace version so team developers see a warning when they check for updates, signaling that they should migrate.
8. As an environment manager, I want to archive a workspace version so it can no longer be pulled, while leaving existing local copies untouched.

## 4. Server administrator

1. As a server administrator, I want to deploy the Nebi server as a binary, container, or Helm chart so I can run it in my organization's infrastructure.
2. As a server administrator, I want to configure the Nebi server through a config file or environment variables so it fits into my existing infrastructure.
3. As a server administrator, I want to configure a production setup with OIDC authentication, Postgres as the database, Valkey as the job queue, Docker as the execution backend, and an enforced environment storage path.
4. As a server administrator, I want the server to expose health checks, metrics, tracing, and structured logging so I can monitor it in production.
5. As a server administrator, I want to lock down the list of available Nebi servers on Nebi clients so I control where workspaces are pulled from and users cannot add, remove, or modify them.
6. As a server administrator, I want to disable standalone operation on Nebi clients so all workspace usage goes through the server.
7. As a server administrator, I want to upgrade the Nebi server to a new version with confidence that existing workspaces, workspace versions, and connected clients will continue to function without data loss or manual intervention.

## 5. Automation / machine consumer

1. As an automation workflow, I want to authenticate to a Nebi server using machine-to-machine OIDC credentials so I can pull workspaces without human interaction.
2. As an automation workflow, I want to pull a specific workspace version so my builds are deterministic.
3. As an automation workflow, I want the Nebi client to operate in a headless mode with structured output, no color, and clean exit codes so I can integrate it into scripts and pipelines.

## 6. Distributed compute operator

1. As a distributed compute operator, I want to configure worker nodes to authenticate to a Nebi server using machine-to-machine OIDC credentials.
2. As a distributed compute operator, I want worker nodes to pull a specific workspace version so the computing environment is identical across all nodes.
3. As a distributed compute operator, I want worker nodes to automatically pull a workspace and execute within its environment so I don't need to manage Docker images or manual synchronization.

## 7. Security & compliance officer

1. As a security officer, I want an audit trail of who pulled which workspace version and when so I can demonstrate compliance during reviews.
2. As a security officer, I want Nebi to expose the resolved package list per workspace version so I can run my own CVE scanner against it and surface the results.
3. As a security officer, I want a per-workspace-version license report listing all packages and their licenses so I can identify problematic dependencies.
4. As a security officer, I want to export a workspace version's package list in a standard SBOM format so I can feed it into compliance tools.
