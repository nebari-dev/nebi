# Nebi Use Cases

This document defines the user stories that Nebi needs to support. There are three primary user personas:

1. **Individual Developer** — works across multiple machines/platforms and needs to sync environments between them.
2. **Team Member** — collaborates within a team and needs to share and consume environments with teammates and across teams.
3. **Organization Admin** — manages the Nebi instance, configures registries, and controls access across teams via a hierarchical permission model.

---

## 1. Individual Developer

### 1.1 Sync environments across machines

**As** an individual developer working on multiple machines (e.g., laptop, workstation, cloud VM),
**I want** to push my pixi environment from one machine and pull it on another,
**so that** I have the same reproducible environment everywhere I work.

**Workflow:**
```bash
# On machine A
nebi login https://nebi.company.com
nebi push my-project:v1.0.0

# On machine B
nebi login https://nebi.company.com
nebi pull my-project:v1.0.0 --install
nebi shell my-project:v1.0.0
```

### 1.2 Version and iterate on environments

**As** a developer,
**I want** to tag versions of my environment as I evolve it,
**so that** I can return to a known-good state or compare what changed.

**Workflow:**
```bash
# Push initial version
nebi push my-project:v1.0.0

# Modify pixi.toml, add packages, etc.
nebi status              # shows "modified"
nebi diff                # shows what changed since v1.0.0
nebi push my-project:v1.1.0
nebi diff my-project:v1.0.0 my-project:v1.1.0  # compare versions
```

### 1.3 Detect local drift

**As** a developer,
**I want** to see whether my local environment has drifted from what I last pulled or pushed,
**so that** I can decide whether to push a new version or reset.

**Workflow:**
```bash
nebi status                 # "clean" or "modified"
nebi status --remote        # also check if remote tag was updated by someone else
nebi diff                   # detailed view of what changed locally
```

### 1.4 Use global environments as reusable tools

**As** a developer,
**I want** to pull environments globally so I can shell into them from anywhere without polluting my project directories,
**so that** I can maintain a library of ready-to-use tool environments.

**Workflow:**
```bash
nebi pull data-tools:latest --global --name data-tools --install
nebi shell data-tools:latest   # works from any directory
```

---

## 2. Team Member

### 2.1 Share an environment with teammates

**As** a team member,
**I want** to push an environment to the server so my teammates can pull it,
**so that** everyone on the team works with the same dependencies.

**Workflow:**
```bash
# Team lead creates and pushes
nebi push team-ml:v2.0.0

# Teammate pulls
nebi pull team-ml:v2.0.0 --install
nebi shell team-ml:v2.0.0
```

### 2.2 Distribute environments via OCI registry

**As** a team member,
**I want** to publish a finalized environment to an OCI registry,
**so that** it can be consumed by CI/CD pipelines, other teams, or external collaborators who don't have direct Nebi server access.

**Workflow:**
```bash
nebi push team-ml:v2.0.0
nebi publish team-ml:v2.0.0 -r ghcr --as myorg/team-ml
```

### 2.3 Consume environments from another team

**As** a team member,
**I want** to pull environments published by other teams,
**so that** I can use their curated toolchains without recreating them myself.

**Workflow:**
```bash
nebi repo list                       # discover available repos
nebi repo info data-eng-tools        # see details, packages, versions
nebi repo tags data-eng-tools        # see available tags
nebi pull data-eng-tools:stable --install
```

### 2.4 Detect when a shared environment has been updated

**As** a team member,
**I want** to know when the remote version of an environment I pulled has been updated,
**so that** I can pull the latest version and stay in sync with my team.

**Workflow:**
```bash
nebi status --remote
# Output: ⚠ Tag 'v2.0.0' has been updated on remote

nebi diff --remote   # see what changed on the server
nebi pull team-ml:v2.0.0 --force --install  # update local copy
```

### 2.5 Manage multiple local copies

**As** a team member working on multiple projects that use different environments,
**I want** to see all my locally pulled environments and their status,
**so that** I can keep track of what I have and clean up stale ones.

**Workflow:**
```bash
nebi repo list --local   # shows all local copies with drift status
nebi repo prune          # clean up entries where directories were deleted
```

---

## 3. Organization Admin

### 3.1 Manage OCI registries

**As** an admin,
**I want** to configure OCI registries that team members can publish to,
**so that** the organization has a controlled set of distribution targets.

**Workflow:**
```bash
nebi registry add prod-ghcr ghcr.io/myorg -u bot-user -p <token> --default
nebi registry add staging-ghcr ghcr.io/myorg-staging -u bot-user -p <token>
nebi registry list
nebi registry set-default prod-ghcr
```

### 3.2 Bootstrap the Nebi instance

**As** an admin,
**I want** to deploy Nebi and create the initial admin user,
**so that** I can onboard teams and start managing environments.

**Workflow:**
```bash
# Set admin credentials on first startup
ADMIN_USERNAME=admin ADMIN_PASSWORD=<secure-password> nebi serve

# Or via Kubernetes
helm install nebi ./chart -n nebi --create-namespace \
  --set auth.adminUsername=admin \
  --set auth.adminPassword=<secure-password>
```

### 3.3 Control who can push, pull, and publish

**As** an admin,
**I want** role-based access control so that team members can only modify their own team's repos, while being able to read repos from other teams,
**so that** environments are shared safely without accidental overwrites.

**Expected permission model:**
- **Read** — pull and shell into environments
- **Write** — push new versions to a repo
- **Publish** — distribute to OCI registries
- **Admin** — manage registries, users, and permissions

### 3.4 Hierarchical permissions with arbitrary nesting

**As** an admin of a large organization with divisions, departments, and sub-teams,
**I want** a hierarchical permission model that supports arbitrary nesting depth (similar to AWS OUs, GCP folders, or Azure management groups),
**so that** each level of the org can self-manage without bottlenecking on a single admin.

**Example structure (arbitrary depth):**
```
acme-corp/                              (org root)
├── research-division/                  (division)
│   ├── ml-department/                  (department)
│   │   ├── nlp-team/                   (team)
│   │   │   ├── nlp-inference/          (sub-team)
│   │   │   └── nlp-training/           (sub-team)
│   │   └── vision-team/
│   └── data-eng-department/
│       ├── pipelines-team/
│       └── platform-team/
├── product-division/
│   ├── backend-team/
│   └── frontend-team/
└── shared/                             (cross-cutting repos)
```

**Permission inheritance model (top-down, additive with deny override):**

- Permissions granted at any level are inherited by all descendants (union/additive model, like GCP/Azure)
- Explicit deny policies override inherited allows at any level (deny always wins)
- Each node has exactly one parent (tree structure, no DAG)
- Admins at a given level can manage everything below them but nothing above or lateral

**Resolution algorithm:**
1. Collect deny policies from root → target node; if any match, reject
2. Collect allow policies from root → target node; if union includes the action, allow
3. Default: deny (secure by default)

**Delegation examples:**
- `acme-corp/` admin can manage all divisions, departments, teams, and repos
- `research-division/` admin can manage all teams and repos within research, but not `product-division/`
- `nlp-team/` lead can add members, create repos, and grant access within `nlp-team/` and its sub-teams
- A member of `nlp-inference/` inherits read access granted at `research-division/` level

**Depth limit:** Support up to 6-10 levels of nesting (practical recommendation: keep to 4-5 for manageability, matching cloud provider best practices)

**Repo namespacing follows the hierarchy:**
```bash
nebi push research-division/ml-department/nlp-team/bert-env:v1.0.0
nebi pull research-division/ml-department/nlp-team/bert-env:v1.0.0

# Or with shorter aliases if the user has context:
nebi push nlp-team/bert-env:v1.0.0   # if unambiguous within their scope
```

### 3.5 Audit and oversight

**As** an admin,
**I want** to see which repos exist, who owns them, and what has been published,
**so that** I can maintain oversight of the organization's environment ecosystem.

**Workflow:**
```bash
nebi repo list              # all repos on the server
nebi repo info <repo>       # detailed info including owner, timestamps
nebi repo tags <repo>       # all published tags with registry info
nebi repo delete <repo>     # remove abandoned repos
```

---

## 4. Cross-Cutting Concerns

### 4.1 Immutable references for reproducibility

**As** any user,
**I want** to reference environments by digest (not just mutable tags),
**so that** I can guarantee I'm using the exact same environment regardless of tag updates.

```bash
nebi pull my-project@sha256:abc123...
```

### 4.2 Offline-friendly workflow

**As** a developer with intermittent connectivity,
**I want** to check status and diff locally without network access,
**so that** I can understand my environment state even when disconnected.

```bash
nebi status        # works offline, compares against .nebi metadata
nebi diff          # works offline, compares local files against stored digests
```

### 4.3 CI/CD integration

**As** a DevOps engineer,
**I want** to use Nebi in automated pipelines with JSON output and non-interactive mode,
**so that** I can script environment setup in CI.

```bash
nebi pull team-ml:stable --yes --install
nebi status --json   # machine-readable output for conditionals
nebi diff --json     # structured diff for automated checks
```

### 4.4 Multi-platform support

**As** a developer working across Linux, macOS, and Windows,
**I want** Nebi to handle platform differences transparently,
**so that** I can push from one OS and pull on another without manual adjustments.

Pixi already handles platform-aware lock files; Nebi stores and distributes these faithfully.

---

## Open Questions

- **Namespace model**: Should repos be namespaced by team/user (e.g., `team-a/my-env`) or flat with ACLs?
- **Permission inheritance**: How do cross-team read permissions work by default? Opt-in or opt-out?
- **Tag mutability**: Should tags be mutable (Docker-style, latest always points to newest) or append-only with explicit overwrite?
- **Garbage collection**: How are old/unused versions cleaned up on the server?
- **Conflict resolution**: What happens when two users push to the same repo:tag concurrently?
