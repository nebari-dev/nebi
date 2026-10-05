# Architecture — workspaces and boundaries

## Workspaces

| Workspace | Package name | Talks to | Ships as |
|---|---|---|---|
| `apps/client` | `@nebi/client` | Client REST API on loopback (session token) | `dist/client`, embedded by `nebi-web` and the desktop app |
| `apps/server` | `@nebi/server` | Team server REST API (OIDC / basic auth) | `dist/server`, embedded by `nebi-server` |
| `packages/ui` | `@nebi/ui` | Nothing — props in, events out | Source package consumed by both apps |

Root `frontend/package.json` declares `"workspaces": ["apps/*", "packages/*"]`
and holds the shared dev tooling (Biome, TypeScript, Vitest, Playwright).
Each workspace has its own `tsconfig`, `vite.config.ts` (apps only),
`vitest.config.ts`, and `components.json`.

## Dependency direction

```
apps/client ──┐
              ├──▶ packages/ui ──▶ @nebari registry components, @nebari/theme
apps/server ──┘
```

- Apps depend on `@nebi/ui`. `@nebi/ui` depends on nothing in the repo.
- Apps never depend on each other.
- Nothing depends on `frontend/src` (legacy).

## Enforced import boundaries

These are lint errors, not suggestions. Configure them with Biome
`noRestrictedImports` overrides in `frontend/biome.json` when scaffolding:

| In | Banned imports | Why |
|---|---|---|
| `packages/ui/**` | `@tanstack/*`, `axios`, `react-router*`, `@nebi/client`, `@nebi/server`, any `api/` path | Presentational only |
| `apps/client/**` | `@nebi/server`, `apps/server/**` | Apps are independent |
| `apps/server/**` | `@nebi/client`, `apps/client/**` | Apps are independent |
| `apps/**`, `packages/**` | anything under `frontend/src` | Legacy is reference only |

Sketch (adjust paths to the final scaffold):

```json
{
  "overrides": [
    {
      "includes": ["packages/ui/**"],
      "linter": {
        "rules": {
          "style": {
            "noRestrictedImports": {
              "level": "error",
              "options": {
                "patterns": [
                  { "group": ["@tanstack/*", "axios", "react-router*"], "message": "packages/ui is presentational: take data as props." },
                  { "group": ["@nebi/client", "@nebi/server", "**/api/**"], "message": "packages/ui must not depend on an app." }
                ]
              }
            }
          }
        }
      }
    }
  ]
}
```

Raw palette classes are caught by a separate check (a GritQL plugin or a CI
grep for `\b(bg|text|border|ring|fill|stroke)-(red|blue|green|yellow|gray|slate|zinc|neutral)-\d`).

## Where does this code go?

| It is… | Put it in |
|---|---|
| A Nebari registry component | `packages/ui/src/components/ui/` via `npx shadcn add @nebari/<name>` |
| A reusable composite used (or likely used) by both apps — status badge, project card, empty state, page header, log viewer | `packages/ui/src/components/<Name>/` |
| A screen | `apps/<app>/src/pages/<Name>/` |
| A composite only one app needs | `apps/<app>/src/components/<Name>/` |
| Data fetching / mutation | `apps/<app>/src/hooks/use<Resource>.ts` |
| Generated API client | `apps/<app>/src/api/` (generated, don't edit) |
| Client-only shared state | `apps/<app>/src/store/` |

When an app composite turns out to be needed by the other app, **move** it to
`packages/ui` in its own PR — don't duplicate it.

## `packages/ui` component contract

- Pure: same props → same output. No fetching, no global stores, no routing.
  Navigation is an `onNavigate`/`href` prop, or a `render` prop for links.
- Typed props; callbacks named `on<Event>`. Loading, empty, and error states
  are props (`status`, `error`), so every state can be rendered and tested.
- Base UI composition: polymorphism through the `render` prop (see
  `nebari-ui` → Composition), never `asChild`.
- Each component ships with a co-located test, and gets a Figma Code Connect
  mapping when its Figma counterpart exists.
- Exported from the package root (`@nebi/ui`). Don't import from internal
  paths.

## The two apps are not one app with a flag

The legacy app branched every screen on `isLocal` / `modeStore` /
`viewModeStore`. The split removes that:

- **Client app:** single user, no login screen, no RBAC gating. It *does* have a
  remote dimension: whether a team Server is configured, reachable, and
  authorized. That is modeled explicitly (see `data-and-state.md`), not as a
  global mode.
- **Server app:** always authenticated, always team. Permissions come from the
  server's responses (roles/capabilities), never from a client-side mode flag.

If a screen seems to need a mode switch, it is probably two screens: one per
app, sharing `packages/ui` composites.
