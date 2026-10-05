---
title: "Nebi Server"
---

The Nebi server is a hosted web interface to manage Nebi projects in a team. It has a similar interface as the local desktop, but with more features for teams and organizations.

This page covers how to run and configure it.

<!-- TODO: Embed video walkthrough of server UI, created with https://github.com/nebari-dev/nebi-video-demo-automation. Update the link in the following iframe. -->

<!-- <iframe width="560" height="315" src="" title="YouTube video player" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" referrerpolicy="strict-origin-when-cross-origin" allowfullscreen></iframe> -->

## Authentication

Nebi does not manage users or passwords. A team server delegates authentication to an OpenID Connect (OIDC) identity provider such as Keycloak, and accepts the access tokens that provider issues:

- The **web UI** signs in with the authorization code flow and PKCE, directly against the identity provider.
- The **CLI** (`nebi login`) and the **desktop app** ("Connect to server") use the OAuth device authorization grant and refresh their tokens automatically.
- The **server** only validates tokens (signature, issuer, audience and expiry). Users are created in Nebi the first time they sign in, and their groups and admin role follow the identity provider.

Configure the provider with these settings (config file keys under `auth:`, or the environment variables shown):

| Setting | Environment variable | Description |
| --- | --- | --- |
| `type` | `NEBI_AUTH_TYPE` | `oidc` (default) or `none` |
| `oidc_issuer_url` | `NEBI_AUTH_OIDC_ISSUER_URL` | Issuer URL; must match the `iss` claim of access tokens |
| `oidc_client_id` | `NEBI_AUTH_OIDC_CLIENT_ID` | Public client used by the web UI, CLI and desktop app; access tokens must list it in `aud` |
| `oidc_scopes` | `NEBI_AUTH_OIDC_SCOPES` | Comma-separated scopes clients request (default `openid,profile,email`) |
| `oidc_admin_groups` | `NEBI_AUTH_OIDC_ADMIN_GROUPS` | Comma-separated identity-provider groups whose members are Nebi admins (default `admin`) |
| `oidc_discovery_url` | `NEBI_AUTH_OIDC_DISCOVERY_URL` | Optional: where Nebi fetches the provider configuration when the issuer URL is not reachable from the server (for example an in-cluster Keycloak service) |
| `jwt_secret` | `NEBI_AUTH_JWT_SECRET` | Secret (32+ characters) that stored registry credentials are encrypted with |

The client in the identity provider must:

- be a **public** client (no secret) with the authorization code flow, PKCE (`S256`) and the device authorization grant enabled
- allow the redirect URI `https://<nebi-host>/auth/callback`, the post-logout redirect URI `https://<nebi-host>/login`, and the web origin `https://<nebi-host>` (the browser calls the provider's token endpoint directly)
- issue **JWT access tokens** that carry the client ID in `aud` and the user's groups in a `groups` claim (a list of group names). In Keycloak, add an *Audience* mapper for the client and a *Group Membership* mapper with *Add to access token* enabled.
- allow the `offline_access` scope, so CLI and desktop logins get a refresh token that outlives the browser session

The [Docker Compose deployment](#docker-compose-deployment) below sets all of this up.

### Running without authentication

Set `NEBI_AUTH_TYPE=none` to turn authentication off. Every request then runs as a single implicit admin user, so only do this on a trusted network, for example for development or evaluation.

```bash
export NEBI_AUTH_TYPE=none
export NEBI_AUTH_JWT_SECRET=replace-with-at-least-32-random-characters
```

## Running the Server

Start the server:

```bash
nebi-server
```

By default (`--host` unset), `nebi-server` binds all interfaces on port `8460` for team deployments.

To use a different port:

```bash
nebi-server --port 9000
```

To explicitly bind a host/interface, use `--host` (or `NEBI_SERVER_HOST`):

```bash
nebi-server --host 127.0.0.1 --port 8460
```

Once the server is running, authenticate from any client machine with [`nebi login`](/cli-team/#connect-to-a-server).

## Docker Compose Deployment

`docker-compose.yml` in the repository runs a complete team deployment for small teams without an existing identity provider: Nebi, Keycloak as the identity provider, Postgres (one database each for Keycloak and Nebi), and Traefik terminating TLS.

- `https://<NEBI_DOMAIN>` serves Nebi.
- `https://<KEYCLOAK_DOMAIN>` serves Keycloak with the realm `nebi`, a preconfigured `nebi` client, and a `nebi-admin` group.

On a host with DNS records for both domains (certificates come from Let's Encrypt):

```bash
cp docker/compose.env.example .env.compose   # fill in the domains and secrets
docker compose --env-file .env.compose up -d
```

On a laptop, use `*.localhost` domains and a throwaway certificate authority instead:

```bash
docker/gen-local-certs.sh                    # then trust docker/certs/ca.crt
# in .env.compose: NEBI_DOMAIN=nebi.nebi.localhost, KEYCLOAK_DOMAIN=auth.nebi.localhost
docker compose -f docker-compose.yml -f docker/local.override.yml --env-file .env.compose up -d
```

Then create users in the Keycloak admin console (`https://<KEYCLOAK_DOMAIN>/admin/`, realm `nebi`) and add admins to the `nebi-admin` group. Group membership changes reach Nebi within a few minutes, the next time a user's access token is renewed.

For a single-user browser UI on your own machine, use `nebi-web` instead. It runs the same embedded React frontend in local mode and binds to `127.0.0.1` by default.

## Background Jobs

Nebi processes background jobs concurrently using an in-memory queue and a worker in the same process. Live logs stream directly from that process; no external queue service or worker deployment is needed. Jobs are not replayed automatically; saved logs remain in the database. Run only one Nebi instance per database.

Job concurrency defaults to half the available CPU cores (at least one parallel job). Set `worker.max_parallel_jobs` in the configuration or `NEBI_WORKER_MAX_PARALLEL_JOBS` to override it with a positive integer.

On startup, pending and running jobs left by the previous process are marked failed, along with projects whose creation or deletion was interrupted. Saved logs remain available, and failed operations can be retried; jobs are not replayed automatically. Run only one Nebi instance per database, since startup recovery assumes the previous instance has stopped.

The server and desktop app allow up to 40 seconds for HTTP shutdown and worker cleanup, including final job status and log writes. New job submissions are rejected during shutdown, and live log streams close once the worker finishes. The supplied Compose deployments allow 45 seconds before forcibly stopping the container.

## API Documentation

The Swagger API docs are available at `http://localhost:8460/docs`.

## Resource Limits

Nebi applies request, admission, and job-runtime limits from the `limits:` config section. Each value can also be overridden with the matching `NEBI_LIMITS_*` environment variable, for example `NEBI_LIMITS_REQUEST_BODY_BYTES`, `NEBI_LIMITS_ACTIVE_JOBS_PER_USER`, or `NEBI_LIMITS_JOB_TIMEOUT_SECONDS`.

Set a numeric limit to `0` to disable that specific guard. Delete jobs are exempt from active-job quotas so users can still remove a project even when pending or running jobs have saturated its quota.

The main job limits are:

- `request_body_bytes`: maximum HTTP request body size.
- `manifest_bytes`, `lock_bytes`, `metadata_bytes`: maximum stored manifest, lockfile, and metadata sizes.
- `package_string_bytes`: package-name size cap.
- `active_jobs_per_user`, `active_jobs_per_project`, `active_jobs_global`: admission quotas for pending/running jobs.
- `job_timeout_seconds`: wall-clock deadline for each job.
- `job_cpu_seconds`: CPU-time budget enforced with `ulimit -t` on Unix.
- `job_storage_bytes`: project storage budget checked during and after jobs.
- `job_log_bytes`: persisted log cap per job.

CPU/file-size setup is fail-closed: if a configured `ulimit` budget cannot be applied, the child command exits with code `125` and Nebi fails the job rather than running unbounded. Storage checks are also fail-closed if the project cannot be walked.

Per-job memory and process-count limits are intentionally left to deployment isolation for now. In Kubernetes or Docker deployments, set Nebi pod/container memory and process limits until Nebi jobs run in isolated execution units.

The HTTP server read timeout is configured separately as `server.read_timeout_seconds` or `NEBI_SERVER_READ_TIMEOUT_SECONDS`. If omitted, Nebi derives it from `limits.request_body_bytes`; set it to `0` to disable.

## Groups

Groups come from the identity provider. Each access token's `groups` claim (a list of group names; Keycloak's leading `/` is stripped) is applied when Nebi first sees the token, and again at least every five minutes:

- For each name in the claim, a group is created in Nebi (if missing) and the user is added to it.
- Memberships in groups that are no longer in the claim are removed.
- The user is an admin exactly when one of their groups is listed in `oidc_admin_groups`.

Groups with zero members are kept so existing project shares survive temporary churn. Admins can share projects with groups and grant groups access to registries; group membership itself is managed only in the identity provider.

## Upgrading from Password Login

Earlier versions of Nebi had built-in users with passwords, admin-managed groups and an identity review queue. When upgrading:

- Password users can no longer sign in. Their projects stay in place. Users signing in through the identity provider are matched only by the provider's issuer and subject, never by username or email, so a new account is created even if a password user with the same name exists.
- Groups created in the Nebi admin UI are deleted, together with their memberships and every project, registry and admin grant they held. Recreate them in the identity provider.
- `ADMIN_USERNAME`, `ADMIN_PASSWORD`, `auth.type: basic`, `auth.oidc_client_secret`, `auth.oidc_redirect_url`, `auth.proxy_admin_groups` (now `auth.oidc_admin_groups`), `auth.proxy_default_role`, `auth.device_flow_client_id` (the web UI, CLI and desktop app all use `auth.oidc_client_id`) and `auth.authorization_stale_after_mins` are no longer supported.
- Log in again with `nebi login` and in the desktop app. Tokens issued by earlier Nebi versions are not accepted.

## What's Next

- See the [CLI Team Workflows](/cli-team/) for push/pull examples
- Check the [CLI Reference](/cli-reference/) for all available commands
