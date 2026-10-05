---
name: nebi-screen
description: >-
  The workflow for building one screen or one shared component of the Nebi
  client/server UI rebuild from a Figma design: read the frame through the
  Figma MCP, extract a behavior spec (using the legacy app as reference only),
  write tests first, build shared primitives in packages/ui, compose the
  screen in its app, then verify visually and with the quality gate. Use when
  asked to "build the <X> screen", "implement this Figma frame", "/nebi-screen
  <figma-url>", "rebuild <page> for the client/server app", or "add <component>
  to packages/ui".
---

# Building a Nebi screen

One screen (or one shared component) per run, and per PR. Follow the phases in
order. **Stop at each checkpoint** — a human approves the plan before code is
written.

Load `nebi-frontend` (house style) and `nebari-ui` (design system) first.

## Inputs

- A **Figma frame URL** for the screen or component. No URL → stop and ask.
  Don't build from screenshots or prose.
- The **target app**: `client`, `server`, or `packages/ui`.
- The **issue** (if one exists) with its acceptance criteria.

## Phase 1 — Read the design

Use the Figma MCP (load the `figma-use` / `figma-implement-design` skill if
your agent ships Figma skills):

1. `get_design_context` on the frame — layout, components, and any Code
   Connect mappings. A mapped node returns a real `@nebi/ui` or Nebari import;
   **use it as given**.
2. `get_screenshot` on the frame — the visual reference for Phase 6.
3. `get_variable_defs` — confirm every color / spacing / radius variable maps
   to a `@nebari/theme` token. A variable with no matching token is a design
   question: list it, don't invent a CSS value.

Treat Figma's generated code as a **description of intent**, not code to
paste. Rewrite it with Nebari components, semantic tokens, and the
`nebi-frontend` structure.

## Phase 2 — Extract the behavior spec

1. Find the screen's rows in `frontend/docs/rebuild/parity-inventory.md`.
2. Read the legacy code they cite (`frontend/src/...`) to understand behavior:
   states, edge cases, API calls and their order, validations, confirmations,
   polling. **Read only — never import or copy it.**
3. Write the spec as a checklist of user-observable acceptance criteria,
   covering the required states in `nebi-frontend` → `references/testing.md`.
   Note anything in Figma that the old app didn't do, and anything the old app
   did that Figma doesn't show (ask: intentional drop, or missing design?).

## Phase 3 — Plan  ⛔ checkpoint

Present, then **wait for approval**:

- The acceptance criteria.
- The component tree: which `@nebi/ui` / Nebari components are reused, which
  are **new** (and whether each belongs in `packages/ui` or the app).
- Hooks and endpoints used (from the generated client); anything missing from
  the API spec.
- Open questions (design gaps, unmapped tokens, behaviors to drop).

If new `packages/ui` components are needed, build those first as **their own
PR** (run this workflow with target `packages/ui`), then come back.

## Phase 4 — Tests first

Turn each acceptance criterion into a failing test (Vitest + Testing Library +
MSW for pages/hooks; props-driven tests for `packages/ui`). Commit-ready but
red.

## Phase 5 — Build

- `packages/ui` components: presentational only, every state renderable from
  props, Base UI `render` composition, exported from the package root.
- Screens: compose `@nebi/ui` + hooks. Data through `use*` hooks over the
  generated client; no fetching in components.
- Semantic tokens only; respect the surface stack (`bg-canvas` page →
  `bg-card` raised → `bg-muted` recessed).
- Make the tests pass. Don't weaken a test to get green — if the spec was
  wrong, say so.

## Phase 6 — Verify

1. Quality gate (`nebi-frontend` → The quality gate). All green, or report
   exactly what failed.
2. Run the app, open the screen in **light and dark**, screenshot it, and
   compare against the Phase 1 Figma screenshot. List visible differences;
   fix the unintended ones, call out the intentional ones.
3. Keyboard pass: tab order, focus visibility, Escape closes overlays.
4. Mark the screen's rows in `parity-inventory.md` as `covered` (or `dropped`
   / `deferred` with a reason).

## Phase 7 — Hand off  ⛔ checkpoint

- New `packages/ui` component with a Figma counterpart → add a Code Connect
  mapping (`figma-code-connect` skill / `add_code_connect_map`) so the next
  screen gets real imports.
- Open the PR (don't merge): Figma link, acceptance criteria with checkboxes,
  before/after screenshots in both themes, parity rows updated, and an
  "AI-assisted" note. Run the `pr-review` skill (frontend lens) if available
  and address its blockers before requesting human review.
- Recurring review comments → propose a lint rule (preferred) or a line in
  `nebi-frontend`, in a separate PR.

## Never

- Import from or paste legacy `frontend/src` code.
- Edit `components/ui/*` or the vendored `nebari-ui` skill.
- Introduce a token, hex color, or raw palette class.
- Skip the Phase 3 checkpoint, or bundle multiple screens into one PR.
