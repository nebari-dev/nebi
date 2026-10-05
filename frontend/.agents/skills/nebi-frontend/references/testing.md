# Testing

## Test-first from the spec

Each screen issue carries a list of acceptance criteria: the behaviors taken
from the Figma frame and the legacy app's behavior. Turn them into failing tests
**before** writing the screen. Old tests in `frontend/src` can remind you of
edge cases, but don't port them. They exercise the old structure.

## What to test where

| Layer | Tool | Test |
|---|---|---|
| `packages/ui` component | Vitest + Testing Library | Renders each state from props (default, loading, empty, error, disabled). Emits the right events. Accessible name and role. |
| App hook | Vitest + MSW | Calls the right endpoint, maps errors to `ApiError`, polling stops on terminal state, mutations invalidate keys |
| App page | Vitest + Testing Library + MSW | Each acceptance criterion: user-visible outcome for each state and interaction |
| Whole app | Playwright | Main flows against a running backend. `@a11y`: axe with zero critical/serious violations in **both themes**. Visual snapshots per theme for finished screens. |

Test behavior a user can see. Avoid implementation details: no snapshot of
the whole DOM, no asserting on class names, no testing that a hook was
called.

## Patterns

Render with providers in one shared helper per app
(`src/test/renderWithProviders.tsx`): a fresh `QueryClient` per test with
`retry: false`, plus a `MemoryRouter` at the route under test.

```tsx
import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw';
import { renderWithProviders } from '@/test/renderWithProviders';
import ProjectList from '.';

it('shows the empty state when there are no projects', async () => {
  server.use(http.get('*/projects', () => HttpResponse.json([])));
  renderWithProviders(<ProjectList />, { route: '/projects' });
  expect(await screen.findByText(/no projects yet/i)).toBeInTheDocument();
});

it('asks for confirmation before deleting', async () => {
  const user = userEvent.setup();
  renderWithProviders(<ProjectList />, { route: '/projects' });
  await user.click(await screen.findByRole('button', { name: /delete demo/i }));
  expect(screen.getByRole('alertdialog')).toBeInTheDocument();
});
```

- Query by role and accessible name first, then label, then text. Use
  `data-testid` only as a last resort.
- Keep MSW handlers per resource in `src/test/handlers/`, typed against the
  generated API types so a spec change breaks the mocks at compile time.
- For any screen that polls, use fake timers (`vi.useFakeTimers({ shouldAdvanceTime: true })`).

## Required states checklist

Every page test covers whichever of these apply:

- [ ] loading (first load)
- [ ] empty
- [ ] error (5xx) with a retry action
- [ ] permission denied (403), server app only
- [ ] unauthenticated (401 redirect), server app only
- [ ] server unreachable / not configured, client app only, for remote data
- [ ] the happy path for each primary interaction, including the confirmation for destructive actions
