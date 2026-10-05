# UI rebuild: working with AI

How the team uses AI agents to rebuild the Nebi UI as separate **client** and
**server** apps (#587) while keeping the result consistent, accessible, and
reviewable.

The rebuild is **greenfield**. All code in `frontend/apps/*` and
`frontend/packages/*` is new. The legacy app in `frontend/src` is reference
material. Agents read it to learn behavior, but nothing is imported or copied
from it.

## Where guidance lives

| Layer | Location | Owner | How it changes |
|---|---|---|---|
| Design system | `frontend/.agents/skills/nebari-ui` | nebari-design upstream | `npm run skills:sync`; never edited here |
| House style | `frontend/.agents/skills/nebi-frontend` | This repo | PR |
| Per-screen workflow | `frontend/.agents/skills/nebi-screen` | This repo | PR |
| Agent map | `frontend/AGENTS.md` | This repo | PR |
| Behavior checklist | `frontend/docs/rebuild/parity-inventory.md` | This repo | Each screen PR updates its rows |
| Visual design | Figma (frames, variables, Code Connect) | Design owner | Figma |

`nebi-frontend` is based on the OpenTeams `frontend-conventions` skill but
stands alone, so contributors don't need user-level skills installed. Where
the two differ, the nebi skill wins.

## The per-screen loop

Run with `/nebi-screen <figma-url>`. The full steps are in the skill.

1. **Read the design** through the Figma MCP: design context, screenshot,
   variables.
2. **Write the behavior spec** from the parity inventory, reading the legacy
   code for reference only.
3. **Plan.** ⛔ A human approves the acceptance criteria, the component tree,
   and the open questions.
4. **Write tests first** from the acceptance criteria.
5. **Build.** New shared components go into `packages/ui` in their own PR
   first.
6. **Verify:** the quality gate, a visual comparison with Figma in both
   themes, and a keyboard pass. Then update the parity rows.
7. **Hand off.** ⛔ Add the Code Connect mapping, open the PR, run the
   `pr-review` skill, then request human review.

## Who does what

| Humans own | AI does |
|---|---|
| Monorepo scaffold and workspace config | Screen and component implementation |
| API contracts and OpenAPI specs | Generating API clients and types from the specs |
| Figma design decisions | Behavior specs drafted from Figma and the legacy app |
| Approving plans; merging PRs | Tests, the parity inventory, audits |
| Changes to `nebi-frontend` rules | Proposing rule or lint changes from repeated review findings |

## PR rules

- One screen, or one shared component, per PR.
- Each PR includes: the Figma link, acceptance criteria as checkboxes,
  light and dark screenshots, and its updated parity rows.
- Note AI assistance in the PR body. A human reviews and merges every PR.
- The client and server apps can progress in parallel, using separate
  worktrees for parallel agents. **Changes to `packages/ui` go through one
  lane at a time** so two agents don't build the same primitive.

## Enforcement (prefer lint over prose)

| Check | Mechanism | Status |
|---|---|---|
| `packages/ui` has no fetching or routing; apps don't import each other; nothing imports legacy `src/` | Biome `noRestrictedImports` overrides (see `nebi-frontend/references/architecture.md`) | Add at scaffold |
| No raw palette classes or hex colors | GritQL plugin or CI grep | Add at scaffold |
| Generated API clients are current | `npm run gen:api` and a git diff check in CI | Add with the first generated client |
| a11y: zero critical/serious axe violations in both themes | `npm run test:a11y` per app | Exists for legacy; extend per app |
| Visual regressions | Playwright screenshots per screen and theme | Add with the first finished screen |
| Vendored `nebari-ui` skill matches upstream | `npm run skills:check` in a weekly workflow | **Done** |
| Periodic UX/a11y sweep | `impeccable` audit and the a11y/UX sections of `web-app-audit`, each milestone; findings filed as issues | Process |

When review catches the same problem twice, add a lint rule (first choice) or
a line in `nebi-frontend` (second choice), in its own PR.

## Figma setup

1. List every route for each app (start from the parity inventory). Create
   one Figma page per app, with one frame per screen and per state.
2. Make Figma variable names match `@nebari/theme` tokens (`canvas`, `header`,
   `card`, `muted`, …). Check with `get_variable_defs`.
3. Map Nebari and `@nebi/ui` components with Code Connect, so
   `get_design_context` returns real imports instead of generated markup.
   Code Connect has the biggest effect on consistency.
4. Where frames don't exist yet, they can be generated from code with
   `figma-generate-design` as a starting point for design to refine.

## Order of work

1. Settle the decisions below.
2. Agent guidance: this doc, the skills, the skill sync. **(this PR)**
3. Scaffold the monorepo (human-led): npm workspaces, shared
   Biome/TS/Vitest/Playwright config, `components.json` per workspace,
   `dist/client` and `dist/server` outputs, import-boundary lint rules.
4. Figma: screen list, variable alignment, Code Connect for the Nebari
   components.
5. Build the `packages/ui` core: app shell, header, sidebar, page header,
   status badges, alerts, data table, empty/error states, log viewer.
6. Build **one reference screen per app** (one list page and one detail
   page). Review them closely; they become the examples `nebi-frontend` points
   to.
7. Build the remaining screens, with the client and server lanes in parallel.
8. Remove `frontend/src` once every parity row is `covered`, `dropped`, or
   `deferred`.

## Decisions

Proposed defaults that agents follow until the team changes them. Record each
outcome here and in `nebi-frontend`.

| # | Question | Proposal | Status |
|---|---|---|---|
| 1 | Client-state library: Zustand (legacy) or Jotai (OpenTeams default)? | **Jotai.** It's greenfield, so there's no migration cost, and it matches the org skill and scaffolds. | Open |
| 2 | File layout: flat kebab-case or PascalCase folders with barrels? | **PascalCase folders + barrel `index.ts`** (OpenTeams default) | Open |
| 3 | API types: how do we get OpenAPI 3 from swag's Swagger 2.0? | Convert in `gen:api` (`swagger2openapi`) for now; revisit when the client API spec is written | Open |
| 4 | Who owns the Nebi Figma file, and where is it? | — | Open |
| 5 | Storybook (or Ladle) for `packages/ui`? | Defer. Props-driven tests and Code Connect first; revisit once `packages/ui` has more than 15 components | Open |
| 6 | Which branch defines legacy behavior: `ux-rework-dev` or `main`? | `ux-rework-dev` (it has the project rename and the split binaries) | Open |
