# Parity inventory: current frontend behavior

This is a checklist of the user-observable behavior in the current single-app frontend (`frontend/src`). The rebuild (`apps/client`, `apps/server`, `packages/ui`) has to account for each item. As the rebuild progresses, move each row's **Status** from `todo` to `covered`, `dropped` (removed on purpose; note why in the PR) or `deferred` (link the tracking issue). The old code is **reference only**. Read it to understand behavior, but never copy or import it into the new apps.

Line references point to `frontend/src/` unless they begin with `e2e/`. "Workspace" was renamed "project" on this branch, so some hooks and props still say `environmentId`.

## Legend

| Column | Values |
| --- | --- |
| Status | `todo` · `covered` · `dropped` · `deferred` |
| Target | `client` (local/desktop UI) · `server` (team UI) · `both` (likely shared via `packages/ui`) · `?` (needs a product decision) |

How Target was chosen: behavior that the old code reaches only when `isLocalMode()` is true, or through `/remote/*`, goes to `client`. Behavior reached only in team mode (auth, sharing, RBAC, admin) goes to `server`. Behavior that runs unconditionally goes to `both`, unless it is a write the read-only server UI shouldn't own; those are marked `?`.

---

## Route: `/login` (`pages/Login.tsx`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| In local mode, `/login` redirects straight to `/projects` and renders nothing | client | todo | `pages/Login.tsx:49-53`, `:132` |
| Gateway auto-login: when `/version` returned a `logout_url`, the browser is sent to the non-API `/auth/session` before the form is shown | server | todo | `pages/Login.tsx:59-76` |
| Right after a logout (`sessionStorage.nebi_logout`), auto-login is skipped once and the form is shown | server | todo | `pages/Login.tsx:62-66`, `components/layout/Layout.tsx:80` |
| Nothing renders until the session check finishes (no form flash) | server | todo | `pages/Login.tsx:135` |
| `?code=` single-use code is exchanged for a JWT (`POST /auth/code/exchange`), stored, then the user is sent to `/` | server | todo | `pages/Login.tsx:81-111` |
| `?error=` from the OIDC callback or interceptor is shown as a message | server | todo | `pages/Login.tsx:87-91` |
| Username/password form (both fields required), "Signing in..." disabled state, on success store the token and user and go to `/` | server | todo | `pages/Login.tsx:113-129`, `:156-194` |
| Error copy mapping: identity review pending (amber warning tone) or rejected, invalid credentials, code exchange failed, login failed, generic | server | todo | `pages/Login.tsx:16-34`, `:157-167` |
| "Sign in with OAuth" button goes to `${apiBase}/auth/oidc/login` | server | todo | `pages/Login.tsx:207-215` |
| Branding logo (light or dark variant) and "Project Management System" tagline | server | todo | `pages/Login.tsx:144-153` |

## Route: `/` (index)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| `/` redirects to `/projects` | both | todo | `App.tsx:111` |

## Route: `/projects` (`pages/Projects.tsx`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Local project list polls every 2s for status updates | both | todo | `hooks/useProjects.ts:5-11` |
| Full-page spinner while projects load, and while the remote list hasn't resolved yet on first load (when connected) | both | todo | `pages/Projects.tsx:245-253` |
| Table columns: Name (plus monospace path for local projects), Status badge, Install-status badge, Size, Created, Actions | both | todo | `pages/Projects.tsx:399-513` |
| Status and install-status badge colors | both | todo | `lib/status.ts:1-28`, `pages/Projects.tsx:435-444` |
| Clicking a row opens `/projects/:id` (local) or `/remote/projects/:id` (remote) | both | todo | `pages/Projects.tsx:416-420` |
| When connected, the list switches between local and remote projects with the header Local/Remote toggle | client | todo | `pages/Projects.tsx:105-151` |
| Remote list polls every 5s, backing off to 30s while errored | client | todo | `hooks/useRemote.ts:143-153` |
| Remote unreachable banner in remote view, which also suppresses the empty state | client | todo | `pages/Projects.tsx:243`, `:295`, `:515` |
| Empty state: "No projects yet. Create your first one!" (hidden while the create form is open) | both | todo | `pages/Projects.tsx:515-521` |
| "New Project" split button. Its menu item "Import Project from Registry" goes to `/registries` | both | todo | `pages/Projects.tsx:264-286` |
| Inline create card with the pixi.toml editor, prefilled with a default manifest | client | todo | `pages/Projects.tsx:52-60`, `:331-397` |
| Validation: the project name comes from the `[workspace] name` line in the TOML. Submit stays disabled until it is non-empty, and a missing name shows an error | client | todo | `pages/Projects.tsx:63-66`, `:155-161`, `:382` |
| Optional local "Path" field, shown only for the local target in local mode | client | todo | `pages/Projects.tsx:354-370` |
| Create target follows the view: in remote view, new projects are created on the remote server (`POST /remote/projects`) and the user stays on the list | client | todo | `pages/Projects.tsx:167-171`, `:268` |
| A local create then opens the new project's detail page on the Jobs tab | client | todo | `pages/Projects.tsx:173-188` |
| Create in team mode, via the same form, creates the project on the server | ? | todo | `pages/Projects.tsx:173-179` |
| Create and delete errors show the API `error` string or a fallback in a red alert | both | todo | `pages/Projects.tsx:195-201`, `:215-222`, `:289-293` |
| Row Install / Uninstall / Retry icon controls (local rows that have `install_status`) | client | todo | `pages/Projects.tsx:457-471` |
| Install/uninstall "job started" notice with "View logs" (opens the detail page's Jobs tab) and dismiss | client | todo | `pages/Projects.tsx:297-329` |
| "Copy pull command" (`nebi login <origin> && nebi pull <name>`) for managed (non-`local`) projects, with a 2s check-mark confirmation | ? | todo | `pages/Projects.tsx:225-236`, `:472-489` |
| Delete with a destructive confirm dialog. The copy names the remote server for remote rows. All delete buttons are disabled while any delete is pending | both | todo | `pages/Projects.tsx:491-507`, `:523-532` |
| Delete routes to the local or remote API depending on the row | client | todo | `pages/Projects.tsx:204-213` |

## Route: `/projects/:id` (`pages/ProjectDetail.tsx`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| The project polls every 2s | both | todo | `hooks/useProjects.ts:13-20` |
| Spinner while loading. A missing project shows a bare "Project not found" (no styled error state) | both | todo | `pages/ProjectDetail.tsx:145-155` |
| Back button to `/projects`, plus title and subtitle | both | todo | `pages/ProjectDetail.tsx:159-171` |
| Header badges: "Local" (source=`local`), status, install status | both | todo | `pages/ProjectDetail.tsx:173-189` |
| Header Install / Uninstall / Retry Install controls (labeled), with a "job started" notice and "View logs" | client | todo | `pages/ProjectDetail.tsx:190-194`, `:246-276` |
| "Use locally" popover for non-local projects: copy `nebi login <origin> && nebi pull <name>`, links to pull docs and install docs, closes on outside click or Esc | server | todo | `pages/ProjectDetail.tsx:195`, `components/project/UseLocallyButton.tsx:13-148` |
| Header "Edit" button lazy-loads pixi.toml, then jumps to the Configuration tab in edit mode. Disabled without `can_write` | ? | todo | `pages/ProjectDetail.tsx:196-224` |
| Read-only notice "You have read-only access…" when `can_write` is false (linked through `aria-describedby`) | both | todo | `pages/ProjectDetail.tsx:236-244` |
| "Publish" button, disabled unless status is `ready` (the tooltip explains why) | ? | todo | `pages/ProjectDetail.tsx:225-229`, `components/publishing/PublishButton.tsx:21-34` |
| "Share" button, shown only to the owner of a non-local project | server | todo | `pages/ProjectDetail.tsx:230-232` |
| Tabs: Overview, Configuration, Versions, Packages, Jobs, Publications (count), Collaborators (count; team mode and non-local only) | both | todo | `pages/ProjectDetail.tsx:278-299` |
| Initial tab can be preset by another page (one-shot pending-tab store) | both | todo | `pages/ProjectDetail.tsx:84-86`, `store/projectNavStore.ts:9-17` |
| Switching tabs clears the page error | both | todo | `pages/ProjectDetail.tsx:280-283` |
| Overview: name, owner badge (falls back to "You" or "Unknown"), status, path and origin (local projects), size, packages count (links to the tab), created, updated, ID with copy button | both | todo | `pages/ProjectDetail.tsx:301-537` |
| Overview: collaborator and group summaries (first 3, then "+N more"). These link to the Collaborators tab in team mode and are plain text in local mode | server | todo | `pages/ProjectDetail.tsx:404-482` |
| Packages tab: table (name, installed version) polling every 2s, with loading spinner and "No packages installed" empty row | both | todo | `pages/ProjectDetail.tsx:539-598`, `hooks/usePackages.ts:4-11` |
| Configuration tab: lazy-loads pixi.toml when the tab opens. Loading spinner, read-only code block, "Failed to load pixi.toml" fallback | both | todo | `pages/ProjectDetail.tsx:110-127`, `:707-729` |
| Configuration edit: Cancel / "Save & Install" runs `PUT pixi-toml` then `POST solve`, shows "Save complete. Install job started" with View logs, and invalidates projects and jobs | ? | todo | `pages/ProjectDetail.tsx:655-705`, `:607-633` |
| Save failure message "Failed to save and install pixi.toml" | ? | todo | `pages/ProjectDetail.tsx:687-689` |
| Versions tab (see Versions section) | both | todo | `pages/ProjectDetail.tsx:732-737` |
| Jobs tab, filtered to this project (see Jobs section) | both | todo | `pages/ProjectDetail.tsx:739-741` |
| Publications tab: cards with a link to the registry UI, Public/Private badge, registry, URL, published by and when, digest. Loading and empty ("No publications yet…") states | both | todo | `pages/ProjectDetail.tsx:743-878` |
| Publications: copy the `nebi import <host>/<ns>/<repo>:<tag>` command (2s "Copied") | both | todo | `pages/ProjectDetail.tsx:129-143`, `lib/registry.ts:7-14` |
| Publications: Make Public / Make Private toggle (`PATCH` publication) | ? | todo | `pages/ProjectDetail.tsx:842-866` |
| Collaborators tab: read-only list of users and groups with role badges, and an empty state | server | todo | `pages/ProjectDetail.tsx:880-890`, `components/sharing/CollaboratorsList.tsx:13-57` |

### pixi.toml editor (`components/project/PixiTomlEditor.tsx`): used by create and edit

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| TOML / UI mode toggle (defaults to TOML) | client | todo | `components/project/PixiTomlEditor.tsx:222`, `:347-374` |
| Unsaved-changes confirm before switching modes. Switching reloads fresh TOML from the server when `onReloadToml` is provided | client | todo | `components/project/PixiTomlEditor.tsx:250-298`, `:338-345` |
| UI mode: project-name input patches `[workspace]/[project] name` | client | todo | `components/project/PixiTomlEditor.tsx:173-193`, `:378-398` |
| UI mode: package table with remove, plus add name and version (Enter submits; an existing name updates its version) that patches `[dependencies]` while keeping comments and other sections | client | todo | `components/project/PixiTomlEditor.tsx:105-171`, `:308-329`, `:399-478` |
| TOML mode: required textarea in a code-block frame, with a "Project will be created as: …" hint, or a warning when the name is missing | client | todo | `components/project/PixiTomlEditor.tsx:481-516` |

## Route: `/remote/projects/:id` (`pages/RemoteProjectDetail.tsx`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Read-only remote project page: back button, "Remote project details (read-only)", Remote and status badges | client | todo | `pages/RemoteProjectDetail.tsx:82-111` |
| Spinner while loading. A missing project shows a bare "Remote project not found" | client | todo | `pages/RemoteProjectDetail.tsx:70-80` |
| Tabs: Overview, Configuration, Version History, Tags. Each tab loads its data lazily on first open | client | todo | `pages/RemoteProjectDetail.tsx:52-68`, `:113-119` |
| Overview: name, owner (if present), status, size in MB (if >0), created and updated (if present), ID with copy | client | todo | `pages/RemoteProjectDetail.tsx:121-237` |
| Configuration: pixi.toml code block, loading, "Failed to load pixi.toml" | client | todo | `pages/RemoteProjectDetail.tsx:239-260` |
| Version History table (project version label, snapshot #, description, created) and "No versions available" | client | todo | `pages/RemoteProjectDetail.tsx:262-312`, `lib/versions.ts:1-6` |
| Tags table (tag, snapshot #, created) and "No tags available" | client | todo | `pages/RemoteProjectDetail.tsx:314-358` |
| No unreachable banner or remote flags on this page (known gap, issue #507) | client | todo | `pages/RemoteProjectDetail.tsx:46-68` |

## Route: `/registries` (`pages/Registries.tsx` → `Registries`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Registry list (name, URL, Default badge, Browse) | both | todo | `pages/Registries.tsx:109-144` |
| "Manage Registries" header button for admins | server | todo | `pages/Registries.tsx:96-104` |
| When connected, the list switches between local and remote registries by view. The remote list retries only while errored | client | todo | `pages/Registries.tsx:54-72`, `hooks/useRemote.ts:202-212` |
| Spinner on first load (local, or remote when connected) | both | todo | `pages/Registries.tsx:77-85` |
| Remote unreachable banner (remote view), which suppresses the empty state | client | todo | `pages/Registries.tsx:74`, `:107`, `:146` |
| Empty state: admins get a link to Admin → Registries, everyone else "Ask an admin to add one." | both | todo | `pages/Registries.tsx:146-162` |
| Browse always goes to `/registries/:id` using the *local* registry endpoints, even when the row came from the remote list | client | todo | `pages/Registries.tsx:133-139` |

## Route: `/registries/:registryId` (`pages/Registries.tsx` → `RegistryRepositories`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Breadcrumb (Registries › name), back button, title | both | todo | `pages/Registries.tsx:436-465` |
| Repository search: the server-side `search` param refetches on every keystroke | both | todo | `pages/Registries.tsx:416-422`, `:467-477` |
| Loading spinner. Fallback warning "Catalog API not available… Showing known publications" when `fallback=true` | both | todo | `pages/Registries.tsx:479-490` |
| Empty: "No repositories found in this registry." | both | todo | `pages/Registries.tsx:516-522` |
| Row: repository, visibility badge (Public / Private / Unknown), tag select (defaults to `latest`, otherwise the first tag), "No tags" | both | todo | `pages/Registries.tsx:194-197`, `:243-291` |
| Row: inline copyable `nebi import` command for the selected tag | both | todo | `pages/Registries.tsx:199-205`, `:292-313` |
| Row: Import / Close toggle opens an inline panel (registry, repo, tag, and a project-name prefill `<repo>-<tag>`). Disabled without a tag | client | todo | `pages/Registries.tsx:207-217`, `:314-405` |
| Import submits `POST /registries/:id/import`, then goes to `/projects`. Errors show inline, and submit is disabled when the name is empty or a request is pending | client | todo | `pages/Registries.tsx:219-237`, `:381-401` |
| The same import flow in team mode creates a server-side project | ? | todo | `pages/Registries.tsx:219-237` |

## Route: `/settings` (`pages/Settings.tsx`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Nav link shown only in local mode (the route itself isn't guarded) | client | todo | `components/layout/Layout.tsx:121-129` |
| "Remote Server Connection" card with a Connected/Disconnected badge | client | todo | `pages/Settings.tsx:81-100` |
| Connected: shows server URL and username, plus Disconnect (pending state). Disconnecting switches the view to local | client | todo | `pages/Settings.tsx:43-54`, `:102-134` |
| Disconnected: URL (type=url), username, password (all required) and Connect (pending state). Success switches the view to remote and clears the form | client | todo | `pages/Settings.tsx:28-41`, `:136-193` |
| Connect and disconnect errors show the API `error` or a fallback | client | todo | `pages/Settings.tsx:37-40`, `:48-53`, `:75-79` |
| Connect and disconnect *reset* (not invalidate) every `['remote']` query, clearing stale errors | client | todo | `hooks/useRemote.ts:116-141` |
| Spinner while the server status loads | client | todo | `pages/Settings.tsx:56-62` |

## Route group: `/admin/*` (`components/layout/AdminLayout.tsx` + `AdminRoute`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Admin gate: probe `GET /admin/users`. Success means admin, failure redirects to `/projects`. Spinner while probing | server | todo | `App.tsx:61-88`, `hooks/useAdmin.ts:10-23` |
| Sidebar: Overview, Users, Groups, Identity Reviews, Registries, Logs, with the active state | server | todo | `components/layout/AdminLayout.tsx:12-59` |
| Admin pages also open in local mode, where the RBAC probe passes. In remote view they proxy `/remote/admin/*` | ? | todo | `App.tsx:61-88`, `hooks/useRemote.ts:226-385` |

## Route: `/admin` (`pages/admin/AdminDashboard.tsx`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Stat cards: Total Users, Environments, Active Jobs (running and pending), Identity Reviews (pending), Disk Usage (or "N/A") | server | todo | `pages/admin/AdminDashboard.tsx:136-145`, `:188-207` |
| "System Alerts" warning that combines failed-job and pending-review counts | server | todo | `pages/admin/AdminDashboard.tsx:172-182`, `:209-216` |
| Quick-action cards linking to Users, Registries, Identity Reviews, Audit Logs | server | todo | `pages/admin/AdminDashboard.tsx:59-84`, `:218-238` |
| Spinner until all queries (including the jobs and projects 2s polls) first load | server | todo | `pages/admin/AdminDashboard.tsx:156-170` |
| Remote view: projects, jobs, stats and reviews come from the remote. Unreachable banner if any required remote query errors. "Total Users" always stays local | client | todo | `pages/admin/AdminDashboard.tsx:95-153`, `:186`, `:190` |

## Route: `/admin/users` (`pages/admin/UserManagement.tsx`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Users table: username with "(you)", email, Admin/User role badge, groups (per-row fetch: "…" while loading, "—" when none, OIDC groups styled blue), created | server | todo | `pages/admin/UserManagement.tsx:25-50`, `:139-247` |
| Grant/Revoke admin with a confirm whose copy depends on the current role. Disabled for yourself, with a tooltip | server | todo | `pages/admin/UserManagement.tsx:184-217`, `:255-276` |
| Delete user with a destructive confirm. Disabled for yourself | server | todo | `pages/admin/UserManagement.tsx:218-241`, `:255-276` |
| Action errors show in a red alert | server | todo | `pages/admin/UserManagement.tsx:91-109`, `:131-135` |
| Create User dialog: username, email, password and confirm (live "Passwords do not match"), "Make Admin". Autofocus, pending state, API error | server | todo | `components/admin/CreateUserDialog.tsx:14-189` |
| Empty: "No users found". Spinner on first load | server | todo | `pages/admin/UserManagement.tsx:111-117`, `:249-253` |
| Remote view lists remote users with an unreachable banner, but grant, revoke, delete, create and the groups cell still hit the *local* API | client | todo | `pages/admin/UserManagement.tsx:59-81`, `:137` |

## Route: `/admin/groups` (`pages/admin/Groups.tsx`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Groups table: name, description, source badge (oidc styled), member count, created | server | todo | `pages/admin/Groups.tsx:71-135` |
| Text loading state ("Loading…") and empty state ("No groups yet.") | server | todo | `pages/admin/Groups.tsx:62-69` |
| Delete group with a destructive confirm. Disabled for OIDC groups, with a tooltip | server | todo | `pages/admin/Groups.tsx:116-129`, `:138-147` |
| Create Group dialog (name required, description) with an API error | server | todo | `components/admin/CreateGroupDialog.tsx:14-97` |
| Members dialog: list (loading or empty), remove member, add member from users not already in the group. Add and remove are hidden for OIDC-synced groups | server | todo | `components/admin/GroupMembersDialog.tsx:32-170` |
| No remote-view branch: always shows local groups | ? | todo | `pages/admin/Groups.tsx:19-21` |
| Group rename/update, grant-admin and revoke-admin exist in the API and hooks but have no UI | ? | todo | `api/groups.ts:24-27`, `:43-48`, `hooks/useGroups.ts:45-55` |

## Route: `/admin/identity-reviews` (`pages/admin/FederatedIdentityReviews.tsx`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Pending/Rejected status tabs | server | todo | `pages/admin/FederatedIdentityReviews.tsx:130`, `:231-239` |
| Table: status badge, existing user (or both users when the collision is ambiguous), incoming identity (name, email, verified badge, sub), collision label, issuer, requested | server | todo | `pages/admin/FederatedIdentityReviews.tsx:46-119`, `:252-412` |
| Pending rows: Reject (destructive confirm) and Approve (confirm). Approve is disabled for ambiguous collisions, with a tooltip. Per-row spinner, and all row actions disabled during a mutation | server | todo | `pages/admin/FederatedIdentityReviews.tsx:266-289`, `:354-388`, `:423-450` |
| Rejected rows: Discard (destructive confirm) | server | todo | `pages/admin/FederatedIdentityReviews.tsx:389-405`, `:452-465` |
| Mutations refresh the reviews and audit logs. Errors show in an alert | server | todo | `hooks/useAdmin.ts:135-175`, `pages/admin/FederatedIdentityReviews.tsx:121-127`, `:245-249` |
| Empty: "No {status} identity reviews". Spinner on first load | server | todo | `pages/admin/FederatedIdentityReviews.tsx:213-219`, `:415-421` |
| Remote view: list and mutations go to `/remote/admin/...`. The table and empty state hide while unreachable | client | todo | `pages/admin/FederatedIdentityReviews.tsx:137-156`, `:170-206`, `:243`, `:251` |

## Route: `/admin/audit-logs` (`pages/admin/AuditLogs.tsx`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Filters: user-ID text (refetches on every keystroke) and action select (11 actions plus All) | server | todo | `pages/admin/AuditLogs.tsx:40-57`, `:117-155` |
| Table: timestamp, user (username or ID), color-coded action badge, resource, expandable "View Details" JSON | server | todo | `pages/admin/AuditLogs.tsx:25-38`, `:157-210` |
| Empty state differs when filters are active | server | todo | `pages/admin/AuditLogs.tsx:212-220` |
| Remote view: remote logs, unreachable banner | client | todo | `pages/admin/AuditLogs.tsx:66-96`, `:115` |

## Route: `/admin/registries` (`pages/admin/RegistryManagement.tsx`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Table: name (star when default), URL, username, Default/Active, Managed and Restricted badges with explanatory tooltips, created | server | todo | `pages/admin/RegistryManagement.tsx:114-217` |
| Edit and Delete are disabled for config-managed registries, with tooltips | server | todo | `pages/admin/RegistryManagement.tsx:176-211` |
| Delete confirm (explains that existing publications stay accessible). Errors show in an alert | server | todo | `pages/admin/RegistryManagement.tsx:69-84`, `:237-246` |
| Add Registry dialog: name, URL, namespace (required), default and restricted checkboxes, optional username, password and API token (Quay hint). Autofocus, API error | server | todo | `components/admin/CreateRegistryDialog.tsx:20-287` |
| Edit dialog prefills fields except the secrets ("leave blank to keep current"), with a "token is currently configured" hint | server | todo | `components/admin/EditRegistryDialog.tsx:53-64`, `:83`, `:256` |
| Empty state ("Add your first registry…"). Spinner on first load | server | todo | `pages/admin/RegistryManagement.tsx:86-92`, `:219-226` |
| Remote view: list, create, edit and delete all target the remote server (`/remote/admin/registries`) | client | todo | `pages/admin/RegistryManagement.tsx:31-58`, `:103`, `:233`, `components/admin/CreateRegistryDialog.tsx:43-45`, `components/admin/EditRegistryDialog.tsx:50` |

---

## Cross-cutting: app shell & navigation

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Blocking spinner until the app mode is resolved from `/version` (3 attempts, 300ms apart) | ? | todo | `App.tsx:34-49`, `store/modeStore.ts:26-67` |
| Runtime base path (`window.__NEBI_BASE_PATH__`) for the router, the API base and assets | both | todo | `lib/basePath.ts:9-15`, `App.tsx:95` |
| Runtime branding from `/public/config.json`, loaded before first render: title, favicon, light/dark logos and theme CSS tokens, sanitized (same-origin or base64 image URLs only, CSP nonce) | both | todo | `main.tsx:21-31`, `lib/brandingConfig.ts:64-89`, `:270-325` |
| Header: brand logo linking to `/projects`, primary nav (Projects, Registries, plus Settings in local mode) with the active state | both | todo | `components/layout/Layout.tsx:91-130` |
| Header Local/Remote view toggle, shown only while a remote server is stored. The choice persists (`nebi-view-mode`, defaults to `remote`) | client | todo | `components/layout/Layout.tsx:132-172`, `store/viewModeStore.ts:11-19` |
| Docs icon opens `https://nebi.nebari.dev/` (Wails `BrowserOpenURL` in desktop, otherwise a new tab) | both | todo | `components/layout/Layout.tsx:174-183`, `lib/openExternal.ts:9-15` |
| Admin shield icon (admins only), active on `/admin*` | server | todo | `components/layout/Layout.tsx:184-195` |
| Profile menu (team mode only): avatar or initial fallback, name and email, theme picker, Sign out. Closes on blur or Esc | server | todo | `components/layout/Layout.tsx:196-203`, `components/layout/ProfileMenu.tsx:17-178` |
| Footer version link (to the commit or the release tag) when `/version` reports a version | both | todo | `components/layout/Layout.tsx:213-228` |
| Admin pages use a full-height sidebar layout; other pages use the padded main area | server | todo | `components/layout/Layout.tsx:207-212`, `components/layout/AdminLayout.tsx:26-67` |

## Cross-cutting: auth, login & session

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Local mode bypasses auth entirely (no login, no token) | client | todo | `App.tsx:51-59`, `api/client.ts:20-24` |
| Token and user persisted in localStorage (`auth-storage` zustand plus raw `auth_token`) | server | todo | `store/authStore.ts:13-31` |
| Bearer token added to every API request | server | todo | `api/client.ts:34-43` |
| A 401 (except `/auth/session`) clears the token and query cache and does a hard redirect to `/login` | server | todo | `api/client.ts:46-67` |
| 403 `identity_review_pending` / `identity_review_rejected` sends the user to `/login?error=<code>` | server | todo | `api/client.ts:15-18`, `:50-56`, `api/client.test.ts:18-34` |
| Logout: clear auth. With a gateway `logout_url`, set `nebi_logout` and go to that URL; otherwise navigate to `/login` | server | todo | `components/layout/Layout.tsx:74-87` |

## Cross-cutting: theme

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Light / dark / system preference persisted under `nebi:themeMode`, following the OS in system mode, toggling `.dark` on `<html>`, with an in-memory fallback when storage throws | both | todo | `hooks/use-theme-preference.ts:41-159`, `lib/theme.ts:5` |
| Pre-paint inline script prevents a flash of the wrong theme | both | todo | `index.html:8-26` |
| Theme picker (menuitemradio) lives only in the team-mode profile menu, so local and desktop users have **no** theme control | both | todo | `components/layout/ProfileMenu.tsx:133-163`, `components/layout/Layout.tsx:196` |
| Logo switches with the resolved theme | both | todo | `lib/brandingConfig.ts:316-325` |
| e2e coverage: system dark honored on login, the picker switches the theme, major pages pass a11y in both themes | both | todo | `e2e/major-pages.spec.ts:115-164` |

## Cross-cutting: jobs, log streaming & notifications (`components/jobs/Jobs.tsx`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Jobs list polls every 2s and can be filtered to one project | both | todo | `hooks/useJobs.ts:4-10`, `components/jobs/Jobs.tsx:173-204` |
| Job card: collapsible (first job expanded), ID, type and status badges (colors per type/status), created timestamp | both | todo | `components/jobs/Jobs.tsx:13-80` |
| Expanded card: project ID, created/started/completed times, logs code block (16 lines, copy button, "Waiting for logs..."), error block, metadata blocks | both | todo | `components/jobs/Jobs.tsx:81-168` |
| Live SSE log streaming (`/jobs/:id/logs/stream`) for pending and running jobs, appended to the stored logs, with a "Live" pulse badge. Stops on `done`/`error` | both | todo | `hooks/useJobLogStream.ts:7-153`, `components/jobs/Jobs.tsx:43-47`, `:71-73`, `:116-121` |
| Streaming requires an auth token, so in local mode (no token) it never starts and only polling updates logs | client | todo | `hooks/useJobLogStream.ts:23-27` |
| Remote jobs: static logs, no SSE. Polls every 5s with backoff. Unreachable banner | client | todo | `components/jobs/Jobs.tsx:43-50`, `:177-228`, `hooks/useRemote.ts:214-224` |
| Empty "No jobs yet". Spinner on first load | both | todo | `components/jobs/Jobs.tsx:209-217`, `:241-245` |
| "Job started" inline notices with "View logs" after install, uninstall and save | client | todo | `pages/Projects.tsx:297-329`, `pages/ProjectDetail.tsx:246-276`, `:607-633` |
| Embedded (iframe, e.g. Jupyter server-proxy) and local mode: polls jobs every 2s and `postMessage`s `nebi:job-completed` to the parent window (same-origin) when a job transitions to completed | client | todo | `hooks/useHostJobNotifications.ts:12-58`, `lib/hostBridge.ts:3-23` |
| Install controls: Install, Retry Install (`install_failed`), disabled Installing/Uninstalling spinner, Uninstall behind a destructive confirm explaining `.pixi/envs` removal. Nothing renders without `install_status` | client | todo | `components/project/InstallControls.tsx:21-119` |

## Cross-cutting: versions (`components/versions/VersionHistory.tsx`)

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Timeline of versions (newest marked "Current"), manifest version label vs "Snapshot N", description, timestamp | both | todo | `components/versions/VersionHistory.tsx:133-295`, `lib/versions.ts:1-6` |
| Empty "No Version History Yet" card. Loading spinner | both | todo | `components/versions/VersionHistory.tsx:106-131` |
| Expand a version to View or Download the lock file and manifest. View opens a blob in a new tab; download saves `pixi-lock-vN.lock` / `pixi-toml-vN.toml` | both | todo | `components/versions/VersionHistory.tsx:58-100`, `:222-263`, `hooks/useVersions.ts:43-89` |
| Rollback to a non-latest version behind a confirm. Disabled unless the project is `ready` (with a hint) | ? | todo | `components/versions/VersionHistory.tsx:49-56`, `:264-284`, `:297-309` |
| `FileViewerDialog` (in-app viewer with copy and download) exists but is unused, along with `useViewLockFile` / `useViewManifest` | ? | todo | `components/versions/FileViewerDialog.tsx:21-89`, `hooks/useVersions.ts:91-125` |

## Cross-cutting: publishing & sharing dialogs

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Publish dialog: registry select (default first), repository with a namespace prefix, tag with an "existing:" hint. Server-computed defaults auto-fill per registry | ? | todo | `components/publishing/PublishDialog.tsx:33-100`, `:190-270` |
| Publish validation ("All fields are required"), submit disabled until complete, pending state, the dialog can't close while pending, API error | ? | todo | `components/publishing/PublishDialog.tsx:102-139`, `:272-310` |
| No registries: "Contact your administrator…" warning | ? | todo | `components/publishing/PublishDialog.tsx:178-188` |
| After a successful publish: success panel, then a full `window.location.reload()` after 2s | ? | todo | `components/publishing/PublishDialog.tsx:120-124`, `:160-171` |
| Share dialog: current access list (users and groups with role badges, remove behind a confirm, the owner can't be removed) | server | todo | `components/sharing/ShareDialog.tsx:194-266`, `:468-477` |
| Share dialog: User/Group mode. Add a user or group as Viewer or Editor. Admins pick from all groups, non-admins from their own groups (`/groups/me`). "All users are already collaborators" / "No groups available" | server | todo | `components/sharing/ShareDialog.tsx:63-150`, `:268-462` |
| The Share dialog's user picker uses admin-only `GET /admin/users`, so a non-admin owner gets no user list | server | todo | `components/sharing/ShareDialog.tsx:65`, `:98-100` |

## Cross-cutting: remote connection & unreachable handling

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| The server-connection status (`GET /remote/server`) polls every 10s with no backoff and is always observed by the shell (the only self-heal for connection status). Enabled only in local mode | client | todo | `hooks/useRemote.ts:42-56`, `components/layout/Layout.tsx:65-68` |
| `status: 'connected'` means credentials are stored, not that the server is reachable | client | todo | `hooks/useRemote.ts:98-114`, `AGENTS.md` (frontend invariants) |
| Remote reachability shows up as query errors (the backend wraps every remote failure as a 502). `isUnreachable` stays true through in-flight retries; `isFirstLoad` gates the spinner so it doesn't flash on retry | client | todo | `hooks/useRemote.ts:58-81` |
| Every banner-feeding query self-heals: `pollWithErrorBackoff` (polls, 30s while errored) or `retryWhileUnreachable` (silent until errored) | client | todo | `hooks/useRemote.ts:19-40` |
| Remote queries pin `notifyOnChangeProps` to avoid re-rendering on every poll tick | client | todo | `hooks/useRemote.ts:83-96` |
| Unreachable banner copy ("Can't reach the remote server… check Settings or disconnect") | client | todo | `components/remote/RemoteUnreachableBanner.tsx:7-15` |
| TanStack `networkMode` is `'always'` in local/desktop (loopback stays reachable while the OS reports offline) and `'online'` in team mode. `refetchOnWindowFocus: false`, `retry: 1` | both | todo | `lib/queryClient.ts:10-34`, `store/modeStore.ts:37`, `:59-60` |
| If `/version` never answers, desktop (Wails runtime present) falls back to local mode and everything else to team | ? | todo | `store/modeStore.ts:14-16`, `:53-66`, `store/modeStore.test.ts:90-108` |

## Cross-cutting: error / empty / loading conventions

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Full-page centered spinner (`h-96`) for the initial page load. Section spinners inside tabs and dialogs | both | todo | e.g. `pages/Projects.tsx:247-253`, `pages/ProjectDetail.tsx:557-560` |
| Page-level red alert that shows the API `response.data.error` or a fallback message | both | todo | e.g. `pages/Projects.tsx:289-293`, `pages/admin/UserManagement.tsx:131-135` |
| Centered muted-text empty states, suppressed while a remote-unreachable banner is showing | both | todo | e.g. `pages/Projects.tsx:515-521`, `pages/Registries.tsx:146-162` |
| Destructive actions use a confirm dialog (alertdialog role, no outside-click dismiss) | both | todo | `components/confirm-dialog.tsx:27-63` |
| Copy-to-clipboard buttons swap to a check mark for 2s | both | todo | e.g. `pages/ProjectDetail.tsx:516-532`, `pages/Registries.tsx:199-205` |
| Not-found states are bare unstyled text (no 404 route, no catch-all) | both | todo | `pages/ProjectDetail.tsx:153-155`, `pages/RemoteProjectDetail.tsx:78-80`, `App.tsx:97-142` |

## Cross-cutting: permissions / RBAC gating in the UI

| Behavior | Target | Status | Old reference |
| --- | --- | --- | --- |
| Admin status is inferred by probing `GET /admin/users` (there is no `is_admin` flag on the session user) | server | todo | `hooks/useAdmin.ts:10-23`, `App.tsx:61-73` |
| Admin-only UI: header shield, "Manage Registries" button, admin link in the registries empty state, group-picker scope in Share | server | todo | `components/layout/Layout.tsx:184-195`, `pages/Registries.tsx:96-104`, `:150-159`, `components/sharing/ShareDialog.tsx:66-75` |
| `can_write` gates Edit configuration, with a read-only notice | both | todo | `pages/ProjectDetail.tsx:107-108`, `:200-201`, `:236-244`, `:640-643` |
| Share is shown only to the owner of a non-local project. The Collaborators tab is hidden in local mode and for local projects | server | todo | `pages/ProjectDetail.tsx:105-106`, `:230-232`, `:294-298`, `:880` |
| You can't toggle admin on or delete yourself. OIDC groups can't be deleted or have members edited. Config-managed registries can't be edited or deleted. Ambiguous identity collisions can't be approved | server | todo | `pages/admin/UserManagement.tsx:195-205`, `:229-237`, `pages/admin/Groups.tsx:119-124`, `components/admin/GroupMembersDialog.tsx:118`, `:136`, `pages/admin/RegistryManagement.tsx:181-207`, `pages/admin/FederatedIdentityReviews.tsx:373-378` |
| Publish and Rollback are gated on project status `ready` (status, not permission) | ? | todo | `components/publishing/PublishButton.tsx:25-30`, `components/versions/VersionHistory.tsx:269-283` |

---

## Mode-specific logic to redesign

Every place the old UI branches on local, team, remote or desktop mode. These branches go away in the split, because each app knows what it is.

- `App.tsx:34-49`: `ModeLoader` blocks all rendering until `/version` resolves the mode.
- `App.tsx:51-59`: `PrivateRoute` skips the auth check when `isLocalMode()`.
- `App.tsx:61-88`: `AdminRoute` decides admin access by probing `/admin/users`, which passes in local mode because RBAC is skipped.
- `store/modeStore.ts:16`: `isDesktopApp()` detects the Wails `window.runtime`.
- `store/modeStore.ts:29-44`: retries `/version` because the desktop embedded server starts asynchronously, then sets mode, features and `logout_url`.
- `store/modeStore.ts:37`, `:59-66`: picks `networkMode` from the mode, and the fallback picks local on desktop or team elsewhere.
- `lib/queryClient.ts:10-21`: starts with `networkMode: 'always'` until the mode is known.
- `store/viewModeStore.ts:11-19`: persisted Local/Remote *view* toggle (a second mode axis, used only when the local app is connected).
- `api/client.ts:20-24`: `redirectToLogin` is a no-op in local mode.
- `hooks/useRemote.ts:42-56`: `useRemoteServer` is enabled only in local mode, because `/remote/*` exists only on the local backend.
- `hooks/useRemote.ts:103-114`: `useRemoteView` derives `isLocalMode`, `viewMode`, `isRemoteConnected` and `isRemoteView`.
- `hooks/useHostJobNotifications.ts:13-14`: host job notifications only when embedded and in local mode.
- `hooks/useJobLogStream.ts:23-27`: implicit. No token in local mode means no SSE streaming.
- `lib/openExternal.ts:9-15`: Wails `BrowserOpenURL` vs `window.open`.
- `components/layout/Layout.tsx:68`: shell subscribes to `useRemoteView`.
- `components/layout/Layout.tsx:121-129`: Settings nav item only in local mode.
- `components/layout/Layout.tsx:133-172`: Local/Remote toggle when connected.
- `components/layout/Layout.tsx:196-203`: profile menu (theme picker and sign out) only in team mode.
- `pages/Login.tsx:49-53`, `:60`, `:82`, `:132`: local mode redirects away and skips the session and code flows.
- `pages/Projects.tsx:76-82`: local and remote project queries plus view flags.
- `pages/Projects.tsx:105-151`: picks the local or remote list by connection and view.
- `pages/Projects.tsx:167-188`: create on the remote vs the local backend.
- `pages/Projects.tsx:204-213`: delete on the remote vs the local backend.
- `pages/Projects.tsx:243-253`: unreachable and first-load gating for the remote list.
- `pages/Projects.tsx:268`: create target defaults to the server in remote view.
- `pages/Projects.tsx:355`: Path field only for the local target in local mode.
- `pages/Projects.tsx:416-420`: row navigation goes to the local or remote detail route.
- `pages/Projects.tsx:457`, `:472-473`: install controls and copy-pull only on local rows.
- `pages/Projects.tsx:528`: delete-confirm copy mentions the remote server.
- `pages/ProjectDetail.tsx:102-104`: `isLocalProject` (source) and `isLocalMode`.
- `pages/ProjectDetail.tsx:173-181`: "Local" badge.
- `pages/ProjectDetail.tsx:195`: "Use locally" only for non-local projects (also shown in local mode, where origin is loopback).
- `pages/ProjectDetail.tsx:230-232`: Share for the owner of a non-local project.
- `pages/ProjectDetail.tsx:294-298`, `:880-890`: Collaborators tab hidden in local mode.
- `pages/ProjectDetail.tsx:404-482`: collaborator and group overview rows link to the tab only in team mode.
- `pages/ProjectDetail.tsx:348-376`: path and origin rows only for local projects.
- `pages/RemoteProjectDetail.tsx:46-68`: separate read-only page for remote projects (raw `useQuery`, no remote flags).
- `pages/Registries.tsx:54-85`: picks the local or remote registry list, with remote unreachable and first-load gating.
- `pages/Settings.tsx:15-54`: connect and disconnect also flip the view mode.
- `components/jobs/Jobs.tsx:43-50`: remote jobs get no SSE and static logs.
- `components/jobs/Jobs.tsx:177-209`: picks the local or remote job list, with unreachable and first-load gating.
- `components/project/InstallControls.tsx:31`: renders nothing without `install_status` (team servers never install).
- `pages/admin/AdminDashboard.tsx:95-162`: picks local or remote projects, jobs, stats and reviews, with unreachable gating.
- `pages/admin/UserManagement.tsx:59-81`: picks the local or remote user list (mutations stay local).
- `pages/admin/AuditLogs.tsx:66-96`: picks local or remote logs.
- `pages/admin/RegistryManagement.tsx:31-58`, `:103`, `:233`: picks local or remote registries and routes mutations to the matching server.
- `components/admin/CreateRegistryDialog.tsx:15-45`: `isRemote` picks the local or remote create mutation.
- `components/admin/EditRegistryDialog.tsx:20`, `:50`: `isRemote` picks the local or remote update mutation.
- `pages/admin/FederatedIdentityReviews.tsx:137-156`, `:170-206`, `:268-288`: picks local or remote reviews and mutations.
- `components/remote/RemoteUnreachableBanner.tsx:7-15`: copy assumes a local-app Settings page.

## API endpoints used

Paths are relative to `${basePath}/api/v1` unless marked. "unused" means it is defined in the API client but not called from any UI.

| Endpoint | Method | Old api file | Target app |
| --- | --- | --- | --- |
| `/version` | GET | `store/modeStore.ts:31`, `hooks/useVersion.ts:14` | both |
| `/public/config.json` (non-API, branding) | GET | `lib/brandingConfig.ts:290-292` | both |
| `/auth/login` | POST | `api/auth.ts:6` | server |
| `/auth/code/exchange` | POST | `pages/Login.tsx:97` | server |
| `/auth/oidc/login` (browser navigation) | GET | `pages/Login.tsx:209` | server |
| `/auth/session` (non-API, browser navigation) | GET | `pages/Login.tsx:71` | server |
| `/projects` | GET | `api/projects.ts:13` | both |
| `/projects` | POST | `api/projects.ts:23` | client (server: ?) |
| `/projects/:id` | GET | `api/projects.ts:18` | both |
| `/projects/:id` | DELETE | `api/projects.ts:28` | client (server: ?) |
| `/projects/:id/pixi-toml` | GET | `api/projects.ts:32` | both |
| `/projects/:id/pixi-toml` | PUT | `api/projects.ts:89` | client (server: ?) |
| `/projects/:id/solve` | POST | `api/projects.ts:93` | client (server: ?) |
| `/projects/:id/install` | POST | `api/projects.ts:98` | client |
| `/projects/:id/uninstall` | POST | `api/projects.ts:103` | client |
| `/projects/:id/versions` | GET | `api/projects.ts:38` | both |
| `/projects/:id/versions/:n` | GET | `api/projects.ts:46` (unused) | ? |
| `/projects/:id/versions/:n/pixi-lock` | GET | `api/projects.ts:56` | both |
| `/projects/:id/versions/:n/pixi-toml` | GET | `api/projects.ts:69` | both |
| `/projects/:id/rollback` | POST | `api/projects.ts:79` | ? |
| `/projects/:id/tags` | GET | `api/projects.ts:84` (unused) | ? |
| `/projects/:id/packages` | GET | `api/packages.ts:6` | both |
| `/projects/:id/packages` | POST | `api/packages.ts:14` (unused) | ? |
| `/projects/:id/packages/:name` | DELETE | `api/packages.ts:18` (unused) | ? |
| `/projects/:id/collaborators` | GET | `api/admin.ts:51` | server |
| `/projects/:id/share` | POST | `api/admin.ts:61` | server |
| `/projects/:id/share/:userId` | DELETE | `api/admin.ts:65` | server |
| `/projects/:id/share-group` | POST | `api/groups.ts:59` | server |
| `/projects/:id/share-group/:groupId` | DELETE | `api/groups.ts:62` | server |
| `/projects/:id/publish-defaults` | GET | `api/registries.ts:56` | ? |
| `/projects/:id/publish` | POST | `api/registries.ts:66` | ? |
| `/projects/:id/publications` | GET | `api/registries.ts:74` | both |
| `/projects/:id/publications/:pubId` | PATCH | `api/registries.ts:83` | ? |
| `/jobs` | GET | `api/jobs.ts:6` | both |
| `/jobs/:id` | GET | `api/jobs.ts:11` (unused) | ? |
| `/jobs/:id/logs/stream` (SSE via `fetch`) | GET | `hooks/useJobLogStream.ts:84-93` | both |
| `/registries` | GET | `api/registries.ts:19` | both |
| `/registries/:id/repositories?search=` | GET | `api/registries.ts:96` | both |
| `/registries/:id/tags?repo=` | GET | `api/registries.ts:107` | both |
| `/registries/:id/import` | POST | `api/registries.ts:117` | client (server: ?) |
| `/groups/me` | GET | `api/groups.ts:51` | server |
| `/admin/users` | GET | `api/admin.ts:18` (also the admin probe) | server |
| `/admin/users` | POST | `api/admin.ts:23` | server |
| `/admin/users/:id/toggle-admin` | POST | `api/admin.ts:28` | server |
| `/admin/users/:id` | DELETE | `api/admin.ts:32` | server |
| `/admin/users/:id/groups` | GET | `api/admin.ts:36` | server |
| `/admin/audit-logs?user_id=&action=` | GET | `api/admin.ts:45` | server |
| `/admin/dashboard/stats` | GET | `api/admin.ts:70` | server |
| `/admin/federated-identity-reviews?status=` | GET | `api/admin.ts:78` | server |
| `/admin/federated-identity-reviews/:id/approve` | POST | `api/admin.ts:87` | server |
| `/admin/federated-identity-reviews/:id/reject` | POST | `api/admin.ts:94` | server |
| `/admin/federated-identity-reviews/:id` | DELETE | `api/admin.ts:100` | server |
| `/admin/groups` | GET | `api/groups.ts:13` | server |
| `/admin/groups` | POST | `api/groups.ts:21` | server |
| `/admin/groups/:id` | GET | `api/groups.ts:17` (unused) | server |
| `/admin/groups/:id` | PATCH | `api/groups.ts:25` (unused) | server |
| `/admin/groups/:id` | DELETE | `api/groups.ts:29` | server |
| `/admin/groups/:id/members` | GET | `api/groups.ts:33` | server |
| `/admin/groups/:id/members` | POST | `api/groups.ts:37` | server |
| `/admin/groups/:id/members/:userId` | DELETE | `api/groups.ts:40` | server |
| `/admin/groups/:id/grant-admin` | POST | `api/groups.ts:44` (unused) | server |
| `/admin/groups/:id/grant-admin` | DELETE | `api/groups.ts:47` (unused) | server |
| `/admin/registries` | GET | `api/registries.ts:25` | server |
| `/admin/registries` | POST | `api/registries.ts:35` | server |
| `/admin/registries/:id` | GET | `api/registries.ts:30` (unused) | server |
| `/admin/registries/:id` | PUT | `api/registries.ts:43` | server |
| `/admin/registries/:id` | DELETE | `api/registries.ts:48` | server |
| `/remote/server` | GET | `api/remote.ts:24` | client |
| `/remote/connect` | POST | `api/remote.ts:29` | client |
| `/remote/server` | DELETE | `api/remote.ts:34` | client |
| `/remote/projects` | GET | `api/remote.ts:39` | client |
| `/remote/projects` | POST | `api/remote.ts:86` | client |
| `/remote/projects/:id` | GET | `api/remote.ts:44` | client |
| `/remote/projects/:id` | DELETE | `api/remote.ts:91` | client |
| `/remote/projects/:id/versions` | GET | `api/remote.ts:49` | client |
| `/remote/projects/:id/tags` | GET | `api/remote.ts:54` | client |
| `/remote/projects/:id/pixi-toml` | GET | `api/remote.ts:59` | client |
| `/remote/projects/:id/versions/:n/pixi-toml` | GET | `api/remote.ts:64` (unused) | client |
| `/remote/projects/:id/versions/:n/pixi-lock` | GET | `api/remote.ts:74` (unused) | client |
| `/remote/registries` | GET | `api/remote.ts:96` | client |
| `/remote/jobs` | GET | `api/remote.ts:102` | client |
| `/remote/admin/users` | GET | `api/remote.ts:108` | ? |
| `/remote/admin/registries` | GET | `api/remote.ts:113` | ? |
| `/remote/admin/registries` | POST | `api/remote.ts:120` | ? |
| `/remote/admin/registries/:id` | PUT | `api/remote.ts:128` | ? |
| `/remote/admin/registries/:id` | DELETE | `api/remote.ts:133` | ? |
| `/remote/admin/audit-logs` | GET | `api/remote.ts:140` | ? |
| `/remote/admin/dashboard/stats` | GET | `api/remote.ts:147` | ? |
| `/remote/admin/federated-identity-reviews?status=` | GET | `api/remote.ts:154-157` | ? |
| `/remote/admin/federated-identity-reviews/:id/approve` | POST | `api/remote.ts:164-166` | ? |
| `/remote/admin/federated-identity-reviews/:id/reject` | POST | `api/remote.ts:171-173` | ? |
| `/remote/admin/federated-identity-reviews/:id` | DELETE | `api/remote.ts:177-179` | ? |
