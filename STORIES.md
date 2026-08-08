# Nebi User Stories

## Stakeholders

| # | Stakeholder | Summary |
|---|---|---|
| 1 | **Developer** | Operates standalone or in a managed org. Creates, pulls, and installs workspaces from local, server, and registry sources. Desktop app and CLI. |
| 2 | **Workspace manager** | Curates which workspaces exist, defines manifests, publishes versions, sets access control and group membership. In larger orgs this is a dedicated role/team. Operates in a managed org. |
| 3 | **Server administrator** | Deploys/configures the Nebi server, manages auth providers, DB, queue, Docker executor, encryption keys. |
| 4 | **Automation / machine consumer** | CI pipelines and other automated workflows. Needs M2M auth, headless client output, deterministic workspace version pinning. |
| 5 | **Distributed compute operator** | Operators of Dask, Ray, Slurm, or similar clusters who need identical environments across all nodes. |
| 6 | **Security & compliance officer** | Needs CVE scanning, SBOM, license tracking, audit trails for workspace usage across the org. |

---

## Glossary

**Workspace** — The manifest (pixi.toml), lockfile (pixi.lock), and optionally any bundled workspace files that together define a reproducible environment. This is what Nebi versions, shares, and publishes.

**Workspace version** — A specific, numbered instance of a workspace within its version lineage.

**Environment** — The materialized set of installed packages on disk, created by Pixi from a workspace.

**Nebi server** — The server component that stores workspaces, authenticates users, and serves workspaces to clients.

**Nebi client** — The CLI or desktop application that connects to a Nebi server or operates standalone. Its behavior can be restricted by server policy.

**Source** — A connection that provides workspaces: a Nebi server, an OCI registry, or the local SQLite database. Multiple sources can be connected simultaneously.

---

## 1. As a **developer**

1. I want to create, list, and delete local workspaces so I can manage my workspace lifecycle.
2. I want to create and use the environments specified in my workspaces.
3. I want to tag workspace versions and diff them so I can mark known-good states and understand what changed between versions.
4. I want to add, list, switch between, and remove sources so I can access workspaces from multiple origins in one place.
5. I want a unified workspace list across all connected sources that shows each workspace's source, install status, and whether it's read-only, so I can discover and browse environments without knowing where they live.
6. I want Nebi to work out of the box without requiring additional setup or infrastructure so I can start immediately.
7. I want to publish a workspace to an OCI source so I or my collaborators can pull it onto another machine.
8. I want to pull a workspace's spec files from any connected source and install its environment — via CLI or a button in the UI — with feedback on progress and where the environment lives on disk. `nebi install` implies a pull when needed.
9. I want to share a workspace with a specific set of team members.
10. I want to manage workspaces through a graphical interface — browsing, editing, installing, and publishing — with the same capabilities for both local and remote workspaces, so the desktop app is a full management interface regardless of where the workspace lives.
11. I want clear explanations when operations fail — with assurance my previous environment is intact — and confirmation before destructive actions, so I can trust Nebi not to lose my work.
12. As a developer on my local machine, I want to log in to a Nebi server through my organization's browser-based identity provider so I can connect to the server.
13. As a developer in a managed environment like JupyterHub, I want Nebi to authenticate using a pre-existing bearer token from my session so I don't have to log in again.
14. As a developer in a managed environment, I want Nebi-managed workspaces to appear as Jupyter kernel options so I can select them without leaving my notebook.
15. I want to check for newer versions of my pulled workspaces — via CLI command or automatic background polling in the UI — so I can decide when to update.

## 2. As a **workspace manager**

1. I want to create, edit, and publish workspace versions — from both CLI and UI — so my team can pull the latest changes.
2. I want each workspace version to have clear lineage, with the ability to deprecate versions (warning on update check) and archive them (block pulls), so I can guide my team toward the right versions.
3. I want to grant and revoke workspace access for users and groups at read-only, edit, or admin levels, so I control who can view, modify, or manage each workspace.
4. I want to add new users to my organization's server.
4. I want to set restrictions on the workspaces created, including specific versions for certain packages and a set of default packages in all workspace.
5. I want to publish a workspace to an OCI source so anyone with that registry connected can discover and pull it as a read-only workspace, making the registry a distribution channel alongside the Nebi server.

## 3. As a **server administrator**

1. I want to deploy the Nebi server — as a binary, container, or Helm chart — and configure it through a config file or environment variables, with production options for OIDC, Postgres, Docker execution, and enforced storage paths.
2. I want the server to expose health checks, metrics, tracing, and structured logging so I can monitor it in production.
3. I want to lock down client behavior — restricting available Nebi servers and disabling standalone operation — so I control where workspaces come from.
4. I want to upgrade the Nebi server to a new version with confidence that existing workspaces, workspace versions, and connected clients will continue to function without data loss or manual intervention.
5. I want Nebi to derive user identities, group memberships, and admin status from the OIDC ID token claims so I don't need to maintain a separate user directory or manage group membership manually within Nebi.

## 4. As an **automation / machine consumer**

1. I want to authenticate to a Nebi server using machine-to-machine OIDC credentials so I can pull workspaces without human interaction.
2. I want the Nebi client to operate in a headless mode with structured output, no color, and clean exit codes so I can integrate it into scripts and pipelines.
3. I want to pull a specific workspace version from any configured source with the same command and output format, so my scripts are deterministic and source-agnostic.

## 5. As a **distributed compute operator**

1. I want to configure worker nodes to authenticate to a Nebi server using machine-to-machine OIDC credentials.
2. I want worker nodes to automatically pull a workspace and execute within its environment so I don't need to manage Docker images or manual synchronization.
3. I want worker nodes to pull a specific workspace version from any source so environments are identical across all nodes.

## 6. As a **security & compliance officer**

1. I want audit trails, package lists for CVE scanning, license reports, and SBOM exports per workspace version, so I can demonstrate compliance.

## Non-functional requirements

- **Accessibility** — The Nebi UI must meet WCAG 2.1 AA and Section 508 compliance.
- **Error resilience** — A failed workspace install must never corrupt or remove a previously working environment.
- **Documentation** — All features must be documented for both end users and administrators.
