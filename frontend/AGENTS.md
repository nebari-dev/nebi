# frontend/AGENTS.md

Guidance for AI coding agents working under `frontend/`. The repo-root
`AGENTS.md` still applies; this file narrows it for the UI.

## The UI is being rebuilt

Nebi's UI is being rebuilt from scratch as separate **client** and **server**
apps that share a component library (issue #587). The work is greenfield:

| Path | What it is | Rules |
|---|---|---|
| `apps/client/` | Local/desktop UI (loopback client API) | New code. Follow `nebi-frontend`. |
| `apps/server/` | Team server UI (always authenticated) | New code. Follow `nebi-frontend`. |
| `packages/ui/` | Shared presentational components | New code. No fetching, no routing. |
| `src/` | **Legacy** single app | Reference only. Read it to learn behavior; never import from it or copy it. Fix bugs here only when asked. |

(Until the workspaces are scaffolded, only `src/` exists.)

## Skills

Load these before writing UI code. They live in `.agents/skills/` (and
`.claude/skills` symlinks to it):

- **`nebi-frontend`**: house style for the new code (layout, boundaries,
  generated API clients, state, connectivity invariants, testing, quality
  gate).
- **`nebari-ui`**: the Nebari design system (components, tokens, surface
  stack, header, motion). Vendored from upstream: **never edit it**. Update it
  with `npm run skills:sync`.
- **`nebi-screen`**: the step-by-step workflow for building one screen or
  shared component from a Figma frame, including the human approval
  checkpoint.

## Design source

Figma is the source of truth for the new screens. Work from a frame URL using
the Figma MCP (`get_design_context`, `get_screenshot`, `get_variable_defs`).
No frame means stop and ask. When a shared component lands in `packages/ui`,
map it with Code Connect.

## Rebuild docs

- `docs/rebuild/README.md`: how the team uses AI on the rebuild, the PR
  rules, and the open decisions.
- `docs/rebuild/parity-inventory.md`: every behavior of the legacy app, with
  its target app and status. Update the rows a PR covers.

## Commands

```bash
npm run dev          # legacy app on :8461 (proxies the backend on :8460)
npm test             # vitest
npm run ci           # biome ci (what CI runs)
npm run test:a11y    # playwright + axe
npm run skills:sync  # re-vendor the nebari-ui skill from nebari-design
npm run skills:check # fail if the vendored nebari-ui skill has drifted
```

Workspace-aware commands are added when `apps/*` and `packages/*` are
scaffolded. Update this section at that point.
