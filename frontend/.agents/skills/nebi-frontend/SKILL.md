---
name: nebi-frontend
description: >-
  Nebi frontend conventions for the client/server UI rebuild — the npm
  workspaces monorepo (apps/client, apps/server, packages/ui), package
  boundaries, generated API clients, TanStack Query and client state,
  connectivity invariants, testing, and the quality gate. Use when writing,
  modifying, or reviewing any code under frontend/apps or frontend/packages,
  when scaffolding a new screen, hook, or shared component, or when deciding
  where a piece of UI code belongs. Pairs with the nebari-ui skill (design
  system) and the nebi-screen skill (per-screen workflow).
---

# Nebi frontend

Nebi's UI is being **rebuilt from scratch** as two apps sharing one component
library (issue #587). All code in `apps/*` and `packages/*` is new. The old
single app in `frontend/src` is **reference material only**.

This skill is the house style for the new code. It is self-contained on
purpose: contributors may not have the OpenTeams `frontend-conventions` skill
installed, and where nebi differs from it, **this skill wins**.

## Who owns what

| Concern | Source of truth |
|---|---|
| Components, tokens, surface stack, header recipe, motion | `nebari-ui` skill (vendored, upstream-managed — never edit) |
| Layout, boundaries, data, state, tests, naming | This skill |
| Per-screen workflow (Figma → spec → tests → build → verify) | `nebi-screen` skill |
| What the old app did, behavior by behavior | `frontend/docs/rebuild/parity-inventory.md` |
| Open team decisions | `frontend/docs/rebuild/README.md#decisions` |

## The layout

```
frontend/
├── apps/
│   ├── client/   # local/desktop UI — single user, loopback client API,
│   │             #   connected/disconnected toward a team server → dist/client
│   └── server/   # team server UI — always authenticated, browse + admin
│                 #   → dist/server
├── packages/
│   └── ui/       # shared design system + presentational components
└── src/          # LEGACY app — read it, never import or copy from it
```

Read `references/architecture.md` before adding a package, a folder, or an
import that crosses a workspace.

## Non-negotiables

1. **Old code is reference, not source.** Never import from `frontend/src`,
   and never copy its components, hooks, stores, or helpers. Read it to learn
   *behavior* (states, edge cases, API order), write that behavior down as an
   acceptance criterion, then implement it fresh.
2. **`packages/ui` is presentational.** Props in, events out. No data
   fetching, no TanStack Query, no HTTP client, no router, no app stores.
3. **Apps never import each other.** Shared code goes to `packages/ui` (UI) or
   a new shared package (non-UI) — agreed in review first.
4. **Nebari first.** Before building any primitive, check the installed
   `components/ui` and the `@nebari` registry (`nebari-ui` skill). Installed
   registry files are upstream-managed: extend at the call site, never edit.
5. **Semantic tokens only.** No raw palette classes (`bg-red-500`,
   `text-gray-900`), no hex values, no invented CSS variables. If a token is
   missing, raise it — don't improvise one.
6. **API types are generated, not hand-written.** Each app talks to its API
   through a client generated from that API's OpenAPI spec
   (`references/data-and-state.md`).
7. **Server data lives in TanStack Query.** Never copy API data into client
   state.
8. **No `any`.** Real types, or `unknown` + narrowing.
9. **Build from a design.** UI work starts from a Figma frame link. No frame →
   stop and ask (see `nebi-screen`).
10. **Run the gate before saying done** (below). Report failures as failures.

## Naming and file layout

> **Proposed — confirm at sync** (see Decisions). Follow it until changed.

- Components and pages: PascalCase folder with the component, its test, and a
  barrel `index.ts`. Import from the folder, never the inner file.
- Hooks: `useThing.ts` (+ `useThing.test.ts`), one resource per file.
- Non-component modules: camelCase (`formatBytes.ts`).
- Domain vocabulary: **project** (not workspace), **Client** for the local
  instance, **Server** for the team instance — always shown by display name.

```
apps/client/src/
├── main.tsx  App.tsx  routes.tsx
├── api/            # generated client + thin wrapper (no hand-written types)
├── hooks/          # useProjects.ts — TanStack Query hooks
├── store/          # client-only state (see data-and-state.md)
├── pages/ProjectList/{ProjectList.tsx, ProjectList.test.tsx, index.ts}
└── components/     # app-specific composites (anything reusable → packages/ui)
```

## The quality gate

Run from `frontend/` (workspace-aware once scaffolded):

```bash
npm run build      # tsc + vite for every workspace
npm test           # vitest, every workspace
npm run ci         # biome ci (format + lint + import boundaries)
npm run test:a11y  # playwright + axe, both themes — zero critical/serious
```

Gotcha: bare `npx tsc --noEmit` checks nothing in a solution-style tsconfig —
use `tsc -b` or `tsc -p <workspace>/tsconfig.app.json --noEmit`.

## Reference files

Read the relevant one before doing that kind of work:

- `references/architecture.md` — workspaces, dependency direction, enforced
  import boundaries, where code goes, what goes in `packages/ui`.
- `references/data-and-state.md` — generated API clients, query hook pattern,
  query keys, client state, connectivity invariants (loopback `networkMode`,
  reachability, unreachable recovery).
- `references/testing.md` — what to test where, Vitest + Testing Library +
  MSW, Playwright a11y and visual snapshots, test-first from the spec.

## What NOT to do

| Don't | Do instead |
|---|---|
| Import or paste from `frontend/src` | Cite it (`path:line`) in the issue/PR, rebuild the behavior |
| Fetch inside `packages/ui` | Take data as props; fetch in the app's hook |
| Edit `components/ui/*` | Wrap or compose at the call site |
| `bg-red-500/10 text-red-500` error banner | Nebari `Alert` with the `destructive` variant |
| Hand-write response types | Regenerate the client from the OpenAPI spec |
| Branch one screen on local vs. team mode | Two apps — each screen belongs to one |
| `useEffect` + fetch | A `use*` hook around `useQuery`/`useMutation` |
| Pin `networkMode: 'online'` in client code | Inherit the client default (`'always'`) |
| Invent a token or color | Ask; tokens come from `@nebari/theme` |
