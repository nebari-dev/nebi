# Remote interface spike

Exploration for the issue "make nebi servers and OCI registries peers":
can one `Remote` interface cover both backends well enough that a client
(CLI, or the web UI in `NEBI_MODE=local`) never needs to know which kind
it is talking to?

**Answer: yes, with adjustments.** The interface proposed in the issue
is close to sufficient. Both backends implement it here (`OCIRemote` in
`oci.go`, `ServerRemote` in `server.go`), and one shared lifecycle
script (`runRemoteConformance` in `conformance_test.go`) passes
unmodified against both:

- a basic-auth OCI registry (`go test ./internal/remote/`, in-memory
  Distribution server), plus an anonymous public-registry variant, and
- a real team-mode nebi server (`go test -tags e2e ./internal/remote/`,
  in-process `server.Run`).

The script exercises the whole client lifecycle: discover auth schemes,
reject bad credentials, authenticate, push a new workspace, list it,
pull latest and pull by ref, hit a push conflict, push a second version,
and get typed not-found errors.

## Changes made to the proposed interface

1. **`context.Context` on every method.** Both backends are network
   clients; non-negotiable in Go.
2. **`Version uint` replaced by string refs.** An OCI registry has no
   monotonic version numbers — a version is a tag, and nebi's default
   tag is the content hash (`sha-<12 hex>`). The server's numeric
   `VersionNumber` stays a backend detail; its tags become the refs.
   Both backends maintain a `latest` ref (the server already upserts a
   `latest` tag on every push; `OCIRemote.Push` adds it as an extra tag
   to match).
3. **`Pull(id)` needs version addressing.** A bare id can only mean
   "latest". The spike overloads the id with an optional `@<ref>`
   suffix; a real API should make it explicit, e.g.
   `Pull(ctx, id, ref string)`.
4. **`Push(Workspace)` works but is under-specified.** The workspace
   must carry its own remote identity: `Ref` (the version being pushed)
   plus `ID` for an existing workspace or `Name` for a new one. That is
   enough, but "push to new" vs "push to existing" being implicit in
   which fields are set is easy to misuse.
5. **Typed errors.** Five sentinels cover everything the client needs
   to react to: `ErrNotAuthenticated`, `ErrUnauthorized`,
   `ErrForbidden`, `ErrNotFound`, `ErrConflict`, matched with
   `errors.Is`. The server side maps cleanly off HTTP status codes
   (`cliclient.APIError`); the OCI side maps oras errors
   (`errcode.ErrorResponse`, `errdef.ErrNotFound`).

## Findings (friction the spike surfaced)

- **`internal/oci` has no typed errors.** Every failure is a bare
  `fmt.Errorf` string. The chain does preserve oras's typed errors via
  `%w`, so `mapOCIError` works — but it works by reaching through
  `internal/oci` into oras internals. Making remotes peers should
  include giving `internal/oci` (or the remote layer) first-class error
  types.
- **`oci.ListRepositories`/`oci.ListTags` cannot reach plain-HTTP
  registries** — unlike the pull/publish paths, they never set
  `PlainHTTP`. The spike lists via oras directly. Worth fixing
  regardless of this issue.
- **OCI push conflict detection is read-before-write.** The server
  rejects an existing tag atomically (409); OCI tag pushes overwrite
  silently, so `OCIRemote.Push` resolves the tag first and a concurrent
  pusher can still win the race. Acceptable for now (conflict
  resolution is out of scope), but "Push errors on conflict" is a
  weaker guarantee on OCI than on the server.
- **Conflict-vs-dedup semantics differ for identical content.** Pushing
  byte-identical content to a new ref: the server deduplicates (new tag,
  same version); OCI creates a new manifest. Pushing to an *existing*
  ref conflicts on both, which is what the interface promises, so this
  stays invisible to the client — but it's worth knowing.
- **`SupportedAuthentication` is configuration, not discovery, for
  OCI.** A registry can't be asked up front which schemes it wants
  (anonymous vs basic only reveals itself as a 401 challenge). The
  spike makes it a constructor flag (`AllowAnonymous`). The server
  *does* have a discovery endpoint (`GET /auth/device-config`), which
  suggests the method is fine but its OCI implementation will be
  config- or probe-based.
- **OIDC device flow doesn't fit `AuthenticationCredential`.** The
  server's third auth path (RFC 8628 device flow) is interactive and
  produces a token out-of-band. It can compose — run the flow outside
  the interface, then `Authenticate` with the resulting token — but the
  credential struct alone can't express it. Token auth against the
  server is demonstrated in `TestServerRemote_TokenAuth`.
- **`Authenticate` means different things.** Server: exchange
  credentials for a session (JWT). OCI: no session exists — validate
  once, then re-send credentials on every call. The interface absorbs
  the difference fine; just don't let clients assume a session.
- **Server workspace creation is asynchronous.** `Push` to a new
  workspace must create it and poll pending → ready before pushing the
  version (`ServerRemote.waitReady`). The synchronous `Push(Workspace)
  error` signature hides a job queue behind a poll loop; a real design
  may want push-to-new to be explicit or async.
- **The server can't distinguish not-found from forbidden.** Pulling an
  unknown workspace ID returns 403, not 404 (RBAC denies before
  checking existence — reasonable anti-enumeration, but it means
  `ErrNotFound` for missing workspaces cannot be promised by every
  remote; the conformance test accepts either).
- **`List` scales differently.** OCI: `/v2/_catalog` + per-repo tag
  list, plus (skipped in the spike) a manifest fetch per repo to filter
  nebi-shaped repositories, and `_catalog` itself isn't universally
  supported (Quay needs its REST API). Server: one `/workspaces` call +
  one tags call per workspace. Fine for a spike; a real client
  probably wants pagination and a cheaper nebi-repo marker on the OCI
  side.
- **Assets are the open modeling question.** OCI bundles carry
  arbitrary asset files; the server's push/pull API only moves
  pixi.toml + pixi.lock. The spike lists asset paths on pull and pushes
  core-only bundles (`PublishPixiOnly`). True peer-ness needs a
  decision: either the server API learns assets, or `Workspace` content
  is defined as core-files-only with assets an OCI extra.

## What was NOT done (deliberately)

- No wiring into `cmd/nebi` or the `/remote/*` handlers — the issue
  only asks whether the interface is viable.
- No nebi-repository filtering in `OCIRemote.List`.
- No Quay token auth, no OIDC device flow inside the interface.
- No asset content transfer.
