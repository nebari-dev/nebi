# Replacing nebi's auth with a standard OIDC relying party

Extended scope for [nebari-dev/nebi#585](https://github.com/nebari-dev/nebi/issues/585):

> Would it break any of nebi's core functionalities if we replace the complete
> auth implementation with an off-the-shelf OIDC relying party implementation?

Analysis is against `ux-rework-dev` at `d02176e`, plus the two external repos
that call nebi's auth endpoints (nebari-nebi-pack, data-science-pack). Claims
marked **(tested)** were checked against the running compose stack in this
directory; everything else is from reading code, with file:line references.

## Short answer

**No core nebi feature depends on the gateway-coupled auth.** Everything nebi
does after login (RBAC, sharing, groups, registry grants, job quotas, audit)
is keyed on local user and group UUIDs. A replacement keeps those as long as
it still resolves the IdP's `(iss, sub)` to a local user.

The replacement does break **three integration contracts** and **one
optional feature**. Each needs a coordinated migration, not a redesign:

1. **data-science-pack's pod tokens.** Its JupyterHub spawner mints a nebi
   token server-to-server by sending a Keycloak ID token as a fake
   `Cookie: IdToken=...` to `GET /api/v1/auth/session`. Removing the cookie
   path breaks nebi inside every Nebari JupyterLab pod.
2. **Nebari's Keycloak client and gateway.** The operator-provisioned client
   only knows the gateway's `/oauth2/callback`. nebi's own callback has to be
   registered, and the gateway's auth for nebi becomes redundant.
3. **The desktop app's "connect to server."** It is password-only. It is
   already broken for OIDC users today, and has to become a device flow.
4. **Password auth (optional feature).** If "complete replacement" drops
   basic auth, the following go with it or need a stand-in:
   - `ADMIN_USERNAME` bootstrap
   - the CLI's password fallback
   - the documented basic-auth deployments
   - the password login behind all 70 CLI e2e tests

One design choice decides whether it breaks more. **nebi should stay the
issuer of its own session tokens** (a standard RP pattern: log in via OIDC,
then run your own session). It should **not** become a pure resource server
that only accepts IdP access tokens. Keycloak access tokens last about 5
minutes, and no nebi client has refresh logic: not the web UI, the CLI, the
desktop remote, or the tokens injected into JupyterLab pods.

## What "off-the-shelf RP" should mean here

| | A. RP + nebi session (recommended) | B. nebi as resource server |
| --- | --- | --- |
| Browser | Auth code + PKCE + nonce against the IdP, then a nebi session (cookie or today's bearer JWT) | Same login, but the SPA holds IdP tokens and refreshes them |
| CLI | Device flow against the IdP, then exchange for a nebi token (today's `/auth/device-token`) | Device flow; store the IdP refresh token; refresh every ~5 min |
| Pods (data-science-pack) | Keycloak token exchange, then the nebi token endpoint | Would need offline tokens or a refresh loop in the pod |
| Changes needed in clients | Login UI only | Every client gains refresh logic |
| Revocation | Unchanged: nebi token TTL (24h today) unless shortened | Near-immediate (access-token TTL) |

Option A keeps every client contract except the ones listed above. Option B
mainly buys faster revocation, and that can be had in A by shortening nebi's
token TTL and adding refresh later. Library-wise, nebi already uses
`coreos/go-oidc` + `x/oauth2`, which is a standard RP building block. The
missing pieces are protocol hygiene and consolidation, not a library.

## What an RP library does not replace

These are nebi-side authorization mapping, not authentication. Any
replacement has to keep them (and ideally run them through **one** function
instead of four copies):

- **Just-in-time user provisioning keyed on `(issuer, subject)`**, with
  collision handling: `findOrCreateFederatedUser`
  (`internal/auth/federated_identity.go:49`). This includes the admin
  **identity review** flow for claims that collide with an existing username
  or verified email.
- **Group sync from the `groups` claim**, including `source=oidc` vs
  `source=native` groups (`internal/auth/group_sync.go`).
- **Admin from IdP groups.** Today it is applied on three of the four login
  paths but not the web callback (README finding 4, tested). A consolidated
  RP fixes this by construction.
- **Fail-closed reconciliation.** Tokens minted from IdP claims are rejected
  if a later group or admin sync failed (`internal/auth/reconciliation_status.go`).
- **The field-encryption key for stored registry credentials.** It is derived
  from `auth.jwt_secret` (`internal/crypto/encrypt.go:32`, `router.go:186`).
  Dropping or rotating that secret as part of a rewrite would make existing
  registry passwords undecryptable, so it needs its own key and a migration,
  or has to stay.

## Impact, by consumer

| Consumer | Today | With option A | Migration |
| --- | --- | --- | --- |
| Local mode (desktop, `nebi-web`, CLI local) | `LocalAuthenticator`, no credentials | Unaffected | None |
| Team RBAC, sharing, groups, registries, quotas, audit | Keyed on user/group UUIDs in casbin (`internal/rbac`) | Unaffected if `(iss, sub)` → user mapping is kept | None |
| Browser, direct OIDC (this compose stack) | Loops without shims; admin groups ignored | Works; one login path | Remove shims |
| Browser, Nebari behind Envoy | Gateway sets `IdToken` cookie; SPA auto-redirects to `/auth/session` | nebi runs its own code flow. With an existing Keycloak session this is a silent redirect: **(tested)** 0.2s, no login form | Register `/api/v1/auth/oidc/callback` on the operator-provisioned client (nebari-nebi-pack `values.yaml` has `redirectURI: /oauth2/callback`); optionally drop gateway auth for nebi. `/api/` is already a `publicRoute`, so the callback is reachable |
| Logout | Assumes gateway `/logout` | RP-initiated logout via the IdP's `end_session_endpoint` | None beyond nebi |
| CLI `nebi login` (device flow) | IdP device flow, then `POST /auth/device-token` exchanges the ID token **(tested, real binary)** | Unchanged | Document the audience requirement (README finding 6) |
| CLI `--username` / password fallback | `POST /auth/login` | Gone if basic auth is dropped | Product decision |
| CLI `--token` / `NEBI_AUTH_TOKEN` | Accepts any nebi JWT; no PATs exist on the server | Unchanged under A | None (PATs are a separate, pre-existing gap) |
| data-science-pack spawner (Nebari JupyterLab pods, jhub-apps env selector) | Keycloak token exchange, then `GET /api/v1/auth/session` with `Cookie: IdToken=<token>`, injected once at spawn as `NEBI_AUTH_TOKEN` (`config/jupyterhub/01-spawner.py`, `_sync_exchange_nebi_id_token_for_jwt` and `_nebi_pre_spawn_hook`) | **Breaks** if the cookie path is removed | Switch the spawner to `POST /api/v1/auth/device-token {"id_token": ...}`, which does the same verify-and-mint and also returns `token` **(tested: the same Keycloak ID token minted a working nebi token through both endpoints)**. Ship the spawner change before removing `/auth/session` |
| Desktop "connect to server" | `POST /remote/connect {url, username, password}`, which calls the remote's password login (`internal/api/handlers/remote.go:47`) | Must become device flow | Already needed today for OIDC users. PR [#163](https://github.com/nebari-dev/nebi/pull/163) had a design (device code proxied through the local backend) |
| Basic-auth deployments (`docker-compose*.yml`, `fly.toml`, `server-setup.md`) | Username/password, `ADMIN_USERNAME` bootstrap | Break if basic auth is removed | Keep basic as a separate mode, or require an IdP. For #585's audience, the compose file in this directory is the replacement |
| Tests | 70 CLI e2e test functions log in with a password in `TestMain`; router tests mint via `BasicAuthenticator` | Need a test IdP if basic auth goes | Mock OIDC provider in e2e, or keep a test-only authenticator |
| Multi-replica | Auth codes and `state` are in memory (`internal/auth/authcode.go`) | Same, unless a shared store is added | None now (#593 moved jobs in-process too) |

## Security gaps a rewrite should close

These gaps exist today regardless of the replacement decision:

- **No PKCE and no `nonce`** on the web code flow (`internal/auth/oidc.go:118`
  calls `AuthCodeURL(state)` only).
- **Web logins never grant or revoke admin from groups**, so admin rights
  outlive removal from the IdP group **(tested)**.
- **Hardcoded scopes** including `groups` (`internal/auth/oidc.go:78`). This
  works on Nebari, where the nebi-pack client already requests a `groups`
  scope, but stock Keycloak rejects it (README finding 5).
- **Password login cannot be turned off** (`auth.type=oidc` panics, #198).
- **Nebi tokens are not tied to the IdP session.** Disabling a user in
  Keycloak leaves their nebi token valid for up to 24h.

## History worth knowing

- [#146](https://github.com/nebari-dev/nebi/pull/146) added `IdToken` cookie
  auth specifically for Envoy Gateway on Nebari.
- [#221](https://github.com/nebari-dev/nebi/pull/221) added `/auth/session`
  outside `/api/`, because Envoy strips OIDC cookies on the public `/api/`
  route.
- [#213](https://github.com/nebari-dev/nebi/pull/213) added the gateway
  logout.
- [#163](https://github.com/nebari-dev/nebi/pull/163) (March 2026, closed as
  superseded by [#220](https://github.com/nebari-dev/nebi/pull/220)) proposed
  exactly this: direct OIDC, removal of the proxy/cookie paths, and a device
  flow for CLI and desktop. It was motivated by Envoy's `ForwardAccessToken`
  overwriting nebi's bearer tokens. #220 shipped only the device-flow part,
  which is why the direct flow and the desktop connect were never finished.

## Suggested sequence

1. Consolidate login paths (web callback, device token, session) behind one
   verify-provision-sync function, which fixes admin sync. Add PKCE, a nonce,
   configurable scopes, and RP-initiated logout. Pure nebi changes; nothing
   external breaks.
2. Make `auth.type=oidc` real (#198): no password route, and the frontend
   goes straight to the OIDC login. Stop inferring "gateway" from "OIDC is
   configured." The shims in `docker-compose.oidc.yml` can then be deleted.
3. Desktop "connect to server" via device flow (salvage from #163).
4. Change the data-science-pack spawner to call `/auth/device-token`, and
   register nebi's callback in nebari-nebi-pack. Then remove the `IdToken`
   cookie middleware and `/auth/session`.
5. Decide separately whether basic auth survives as its own mode.

## Open questions

- Should basic auth stay for single-user or evaluation installs, or should
  every team install need an IdP? This changes the test strategy more than
  the code.
- Is a 24h nebi token acceptable once nebi is the only auth layer? If not,
  shorten it and add refresh to the CLI and desktop remote, but not to pods,
  which get their token once at spawn.
- Does anything else call `/api/v1/auth/session` with a cookie? I only
  checked nebari-nebi-pack and data-science-pack.
