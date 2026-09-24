# Nebi + Keycloak via Docker Compose (OIDC only)

Research spike for [nebari-dev/nebi#585](https://github.com/nebari-dev/nebi/issues/585):
can a team of roughly 5 to 25 people run a nebi server alongside Keycloak with
plain Docker Compose, with nebi doing no user management of its own?

**Answer: yes, with no nebi code changes today**, but only with three
reverse-proxy shims and a Keycloak bootstrap step that work around gaps in
nebi. Those gaps are small, well-contained code changes (listed under
[Recommended changes](#recommended-changes)); once made, the shims can go.

## What's here

| File | Purpose |
| --- | --- |
| `../../docker-compose.oidc.yml` | Traefik, Postgres, Keycloak, a one-shot Keycloak bootstrap, nebi |
| `keycloak/nebi-realm.json` | Realm `nebi`: clients `nebi` (web) and `nebi-cli` (device flow), groups `nebi-admin` and `data-science` |
| `keycloak/bootstrap.sh` | Idempotent `kcadm` step that adds the `groups` client scope (see finding 5) |
| `postgres-init.sh` | Creates the `keycloak` and `nebi` databases and roles on first boot |
| `local.override.yml`, `traefik/tls.yml`, `gen-local-certs.sh` | Laptop mode: `*.nebi.localhost` with a throwaway CA instead of Let's Encrypt |
| `.env.example` | Every variable the stack needs |

Topology:

```
                 ┌──────────── traefik :443 ────────────┐
 browser / CLI → │ nebi.example.com   → nebi:8460       │
                 │ auth.example.com   → keycloak:8080   │
                 └──────────────────────────────────────┘
 nebi ──(discovery, token, JWKS over the compose network)──→ http://keycloak:8080
 keycloak, nebi ──→ postgres (databases "keycloak" and "nebi")
```

nebi validates tokens against the public issuer
(`https://auth.example.com/realms/nebi`) but fetches discovery from
`http://keycloak:8080` (`NEBI_AUTH_OIDC_DISCOVERY_URL`). Keycloak's
`KC_HOSTNAME_BACKCHANNEL_DYNAMIC=true` then hands nebi internal token and JWKS
URLs. So nebi never hairpins through the public hostname and never has to trust
the public certificate.

## Running it

Public host (DNS A records for both subdomains pointing at the host, ports 80
and 443 open):

```bash
cp docker/oidc/.env.example .env.oidc     # fill in; secrets via `openssl rand -hex 32`
docker compose -f docker-compose.oidc.yml --env-file .env.oidc up -d
```

Laptop:

```bash
docker/oidc/gen-local-certs.sh            # CA + *.nebi.localhost cert in docker/oidc/certs/
# .env.oidc: NEBI_DOMAIN=nebi.nebi.localhost, KEYCLOAK_DOMAIN=auth.nebi.localhost
docker compose -f docker-compose.oidc.yml -f docker/oidc/local.override.yml \
  --env-file .env.oidc up -d
```

Then create users in the Keycloak admin console
(`https://auth.<domain>/admin/`, realm `nebi`). Put admins in the
`nebi-admin` group, and **have each admin run `nebi login https://nebi.<domain>`
once** (finding 4).

## What was verified

Run on 2026-09-24 against `main` at `07d4fc3`, with the image built from this
tree, Keycloak 26.4, and Postgres 18:

| Check | Result |
| --- | --- |
| Keycloak on Postgres, realm import, restart is idempotent | ✅ |
| Browser: `https://nebi.<domain>/` → Keycloak login → back in nebi (headless Chromium) | ✅ with shim 1; ❌ infinite redirect loop without it |
| Keycloak groups synced into nebi as `source=oidc` groups | ✅ |
| Removing a user from a Keycloak group removes the nebi membership on next login | ✅ |
| `nebi login` (real CLI binary) via RFC 8628 device flow | ✅ (needs audience mapper, finding 6) |
| Admin via `nebi-admin` group | ⚠️ only after a CLI login, never from a web login (finding 4) |
| Logout ends the Keycloak session; next login prompts again | ✅ with shim 2 |
| Password login refused | ✅ with shim 3 (403 at Traefik); ❌ without it |
| Workspace create + pixi install job completes | ✅ |
| Data survives `docker compose down && up` | ✅ |

## Findings

1. **`NEBI_AUTH_TYPE=oidc` panics on startup** (reproduced: nil pointer at
   `internal/api/router.go:205`, tracked in
   [#198](https://github.com/nebari-dev/nebi/issues/198)). The router only
   builds an authenticator for `basic`, so OIDC is always *layered on*
   basic auth, and `POST /api/v1/auth/login` stays live. An admin can still
   create password users (`POST /api/v1/admin/users` works) and they can log
   in. **Shim 3** refuses that endpoint at Traefik.

2. **The login page loops forever when OIDC is configured without an auth
   gateway.** Setting `oidc_issuer_url` + `oidc_client_id` makes `/version`
   return `logout_url: "/logout"`. The frontend reads that as "an
   Envoy-style gateway is in front of me" and sends the browser to
   `/auth/session`, which looks for an `IdToken*` cookie, finds none, and
   302s back to `/login`. Measured: ~250 round trips in 8 seconds, and the
   "Sign in with OAuth" button never renders. **Shim 1** redirects
   `/auth/session` to `/api/v1/auth/oidc/login` (nebi's own authorization-code
   flow), which also turns "open nebi" into "land on Keycloak" and so gives
   the OIDC-only UX.

3. **Logout assumes a gateway.** The logout button navigates to `/logout`,
   which only exists on Envoy Gateway. Without it, nebi serves the SPA and the
   Keycloak SSO session survives, so "Sign in" logs you straight back in.
   **Shim 2** redirects `/logout` to Keycloak's end-session endpoint.

4. **Web (authorization-code) logins never apply `proxy_admin_groups`.**
   `OIDCAuthenticator.loginWithVerifiedClaims` syncs groups but not the admin
   role. Only the device-flow exchange, `/auth/session`, and the `IdToken`
   cookie middleware call `syncProxyAdminRole`. Admins also cannot use "grant
   admin to group" on an OIDC group (`409: cannot grant admin to an
   OIDC-synced group`). Consequences, all reproduced:
   - **Bootstrap:** someone in `nebi-admin` has no admin rights until they run
     `nebi login` from the CLI once.
   - **Revocation (security relevant):** a user removed from `nebi-admin` in
     Keycloak **keeps admin** across any number of web logins. Admin is
     revoked only when that user next does a CLI device-flow login. (Once
     revoked it applies at once, even to existing tokens, because admin is
     checked live in casbin.)

   No shim covers this. Operators must know about it, or it gets fixed in
   code (a one-line change, see below).

5. **nebi hardcodes `scope=openid profile email groups`.** Stock Keycloak has
   no `groups` client scope, so it rejects the request with `invalid_scope`,
   and nebi's callback then answers with a bare JSON
   `400 missing authorization code` instead of sending the user back to the
   login page with an error. Declaring `clientScopes` in the realm import
   stops Keycloak from creating its built-in scopes (profile, email, and so
   on), which is why `keycloak/bootstrap.sh` adds the scope after import.

6. **The CLI's device-flow ID token must carry the web client's audience.**
   `/auth/device-token` verifies `aud` against `NEBI_AUTH_OIDC_CLIENT_ID`
   (`nebi`), but the token is issued to `nebi-cli`. The realm adds an
   audience mapper to `nebi-cli` (the token gets `aud: ["nebi-cli","nebi"]`).
   Nothing in nebi's docs mentions this.

7. **Cosmetic:** after logout the login page still shows the
   username/password form, which is dead behind shim 3.

8. Smaller notes:
   - `NEBI_AUTH_DEVICE_FLOW_CLIENT_ID` and `NEBI_AUTH_PROXY_ADMIN_GROUPS` do
     work as environment variables, but neither they nor
     `NEBI_AUTH_OIDC_DISCOVERY_URL` appear in `nebi serve --help`.
   - Nebi JWTs live 24h and are not tied to the Keycloak session. Disabling a
     user in Keycloak does not end their current nebi session. Group and admin
     changes land on the next login.
   - The stack runs nebi as a single `--mode=both` container with the memory
     queue. For 5 to 25 users that is enough. Valkey and extra workers from
     `docker-compose.prod.yml` can be added unchanged.
   - Unrelated, spotted while testing: the theme pre-paint `<script>` in
     `frontend/index.html` gets no CSP nonce in team mode, so the browser
     blocks it on every page load.

## Recommended changes

These would make "OIDC only" a first-class mode and remove every shim from
this compose file:

1. **Make `auth.type=oidc` real** (fixes #198). Don't register
   `POST /auth/login`, skip `ADMIN_USERNAME` bootstrap, and advertise the mode
   in `/version` so the frontend hides the password form and redirects
   straight to `/api/v1/auth/oidc/login`. Removes shims 1 and 3 and finding 7.
2. **Stop inferring "behind a gateway" from "OIDC configured."** Make it
   explicit (for example `auth.gateway_logout_url`), leave it unset for direct
   OIDC, and do RP-initiated logout via the provider's `end_session_endpoint`
   otherwise. Removes shim 2 and fixes finding 2 for every non-gateway
   deployment, not just this one.
3. **Call `syncProxyAdminRole` from `loginWithVerifiedClaims`** so web logins
   grant and revoke admin like the other three paths. Fixes finding 4. This
   is the one I'd do first, since it is a silent privilege-retention bug in
   any direct-OIDC deployment, compose or not.
4. **Make OIDC scopes configurable** (default without `groups`, or retry
   without it), and redirect to `/login?error=...` when the callback carries
   `error=`. Removes `bootstrap.sh`.
5. Document the device-flow audience requirement and the three missing env
   vars in `nebi serve --help` and the server-setup docs.
