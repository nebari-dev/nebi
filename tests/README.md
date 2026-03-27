# End-to-End Tests

This directory contains end-to-end tests written with [Playwright](https://playwright.dev/).

## Running Tests

Make sure the dev server is running on `http://localhost:8461` before executing tests.

```bash
# Run all tests
npx playwright test

# Run a specific spec file
npx playwright test tests/workspaces.spec.ts

# Run with the Playwright UI
npx playwright test --ui

# View the last HTML report
npx playwright show-report
```

## Configuration

See [`playwright.config.ts`](../playwright.config.ts) for project configuration, base URL, and browser setup.

## Authentication

`auth.setup.ts` runs first and saves authenticated browser state to `playwright/.auth/user.json`. All subsequent tests reuse this stored session so login only happens once per run.
