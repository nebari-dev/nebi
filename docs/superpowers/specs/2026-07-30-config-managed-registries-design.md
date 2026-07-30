# Config-Managed OCI Registries

**Issue:** https://github.com/nebari-dev/nebi/issues/475
**Date:** 2026-07-30
**Status:** Approved

## Problem

Admins deploying nebi through the data-science-pack (where nebi runs in local
mode, one instance per JupyterHub user pod, each with its own SQLite database)
have no way to provision OCI registries for all users. Registries can only be
created through the admin UI/API/CLI against a single instance's database.
Additionally, the built-in default registry (`quay.io/nebari_environments`) is
re-seeded on every boot, so it cannot be removed even deliberately.

Requirements from the issue:

1. Admins can define OCI registries for all users via YAML configuration that
   persists across redeploys.
2. Admins can remove the built-in default OCI registry.
3. Shared credentials (one username/password for all users) are acceptable
   short-term; per-user credentials are future work.

## Solution overview

Add a declarative `registries` section to nebi's `config.yaml`. At boot, nebi
reconciles these entries into the existing `oci_registries` table, marks them
config-managed, and locks them against UI/API/CLI modification. A separate
`seed_default` flag disables the built-in default registry seed, and the
seeder gains a one-time marker so a deliberately deleted default stays
deleted. The data-science-pack chart renders this config into a Secret
mounted at `/etc/nebi/config.yaml` in every user pod (a path nebi already
searches).

Reconciling into the DB (rather than serving registries from the file at read
time) was chosen because every existing consumer resolves registries from the
DB: `Publication.RegistryID` is a real foreign key, the OCI layer loads
credentials from DB rows, and the publish flow queries `is_default`. File-only
registries would require special-casing every consumer.

## 1. Config schema

New optional section in `config.yaml`, parsed in `internal/config`:

```yaml
registries:
  seed_default: false        # default true; false disables the built-in
                             # quay.io/nebari_environments seed
  entries:
    - name: acme-registry    # required, unique; identity key for reconciliation
      url: registry.acme.com # required
      namespace: acme-envs
      username: shared-user
      password: ${ACME_REGISTRY_PASSWORD}
      api_token: ""
      default: true          # at most one entry may set this
```

Validation at `config.Load()`:

- `name` required and unique across entries; `url` required.
- At most one entry with `default: true`.
- `${VAR}` environment expansion applies to `username`, `password`, and
  `api_token` only. Referencing an unset environment variable is a load
  error: fail loud at boot rather than silently operate with an empty
  credential.

This is the first list-valued config block; it is YAML-file-only. The
`NEBI_<SECTION>_<KEY>` env override mechanism does not apply to the entries
list (documented in `config.yaml.example`).

## 2. Boot-time reconciliation and locking

**Model change:** new bool column `config_managed` on `oci_registries`
(handled by gorm auto-migration), serialized as `config_managed` in API
responses.

**Reconciler:** runs at boot in both team and local mode, after `db.Migrate`,
in the service layer (it needs the config and the existing AES-GCM encryption
key derived from the JWT secret):

- **Upsert by name.** Each config entry is created or updated, credentials
  encrypted with the existing scheme. If the name collides with a
  user-created registry, config wins: the row is taken over, marked
  config-managed, and a warning is logged.
- **Remove absent entries.** Config-managed rows whose name no longer appears
  in config are deleted, with the same hard-delete semantics as today's admin
  delete (existing publications keep a dangling `RegistryID`, matching
  current behavior).
- **Default flag.** An entry with `default: true` becomes the default,
  clearing the flag on all other rows (reusing the existing single-default
  convention). If no config entry claims default, existing default state is
  untouched.

**Locking:** `UpdateRegistry` and `DeleteRegistry` in
`internal/service/registry.go` reject config-managed rows with a typed
"managed by server configuration" error. The API maps it to HTTP 409; the CLI
surfaces the message; the admin UI disables edit/delete for those rows and
shows a badge with a tooltip.

**Scope caveat (documented, accepted):** in local mode the lock is a
consistency mechanism, not a security boundary. The per-user nebi process is
owned by the user, who could shadow `/etc/nebi/config.yaml` with a
`config.yaml` in the working directory or manipulate their own database. This
matches the issue's threat model: all users are entitled to the shared
credentials.

## 3. Default-seed flag and re-seed fix

- `seedDefaultRegistry` (`internal/db/db.go`) runs only when
  `registries.seed_default` is true. The flag defaults to true, so existing
  deployments see no change.
- **Re-seed fix:** new minimal `system_settings` key/value table. Seeding
  writes a `default_registry_seeded` marker; the seeder skips when the marker
  exists. An admin who deletes the default registry no longer sees it
  resurrected on restart. For existing databases that already contain the
  `nebari-environments` row, the marker is backfilled on first migration so
  behavior is stable.
- If seeding is disabled and no config entry is marked default, no default
  registry exists. Publishing then follows the existing "no default registry"
  error path (`GetPublishDefaults` returns not-found). This is the admin's
  deliberate choice; no new handling required.

## 4. data-science-pack chart wiring

Separate PR against https://github.com/nebari-dev/data-science-pack:

- New values under the existing `nebi:` block in `values.yaml`:
  - `nebi.seedDefaultRegistry` (bool, default true)
  - `nebi.registries` (list mirroring the nebi config entry fields)
- When either is customized, the chart renders a nebi `config.yaml` into a
  **Secret** (credentials may be inline) and the spawner (`01-spawner.py`,
  which already manages singleuser pod volumes) mounts it at
  `/etc/nebi/config.yaml`. nebi already searches `/etc/nebi/`, so no
  discovery changes are needed on the nebi side.
- Values comments and pack docs note that credentials mounted into user pods
  are readable by those users.

## Error handling

- Invalid registries config (duplicate names, missing url, two defaults,
  unset `${VAR}`) fails `config.Load()` with a descriptive error; nebi does
  not start.
- Reconciler failures (e.g. DB errors) fail startup the same way migration
  failures do today.
- Removal of a config-managed registry that publications reference follows
  existing delete semantics (allowed; publications retain the ID).
- API mutation of a config-managed registry returns 409 with a stable error
  message the CLI passes through.

## Testing

- **Config:** parsing, validation errors, `${VAR}` expansion including the
  unset-variable failure.
- **Reconciler:** create, update-in-place, takeover of a same-named
  user-created registry, removal when dropped from config, default
  switching, credentials encrypted at rest.
- **Locking:** service-level update/delete rejection; handler test asserting
  409; list responses include `config_managed`.
- **Seed:** flag disables seeding; marker prevents re-seed after delete;
  marker backfill for existing databases.
- **Frontend:** admin registry table renders the managed badge and disables
  edit/delete (component test).
- **Pack:** helm-template test asserting the Secret renders and the spawner
  mounts it when `nebi.registries` is set, and that nothing renders when
  unset.

## Out of scope / future work

- Per-user registry credentials (users supplying their own username/password
  for an admin-defined registry) — noted in the issue as the desired
  long-term direction.
- Team-mode-specific UI for editing config-managed registries.
- Env-var override support for the entries list.
