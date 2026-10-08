import { expect, test } from '@playwright/test';
import {
  expectNoCriticalOrSeriousA11yViolations,
  expectResolvedTheme,
  fulfillJson,
  mockApi,
} from './helpers';

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

test('client bypasses login and exposes local settings', async ({ page }) => {
  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible();
  await expect(page.getByPlaceholder('Password')).toHaveCount(0);
  await expect(page.getByRole('button', { name: /testuser/i })).toHaveCount(0);
  await page.getByRole('link', { name: 'Settings', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
  await expect(page.getByText('Remote Server Connection')).toBeVisible();
});

test('client keeps its remote view and unreachable banner', async ({
  page,
}) => {
  await page.route('**/api/v1/remote/server', (route) =>
    fulfillJson(route, {
      status: 'connected',
      url: 'https://remote.example.com',
      username: 'testuser',
    }),
  );
  await page.route('**/api/v1/remote/projects', (route) =>
    fulfillJson(route, { error: 'Remote unavailable' }, 502),
  );
  await page.goto('/projects');
  await expect(page.getByText(/Can't reach the remote server/)).toBeVisible();
  await page.getByRole('button', { name: 'Local', exact: true }).click();
  await expect(page.getByText('analytics-project')).toBeVisible();
  await expect(page.getByText(/Can't reach the remote server/)).toHaveCount(0);
});

for (const theme of ['light', 'dark'] as const) {
  test(`client connection and remote detail pages pass a11y checks in ${theme} mode @a11y`, async ({
    page,
  }) => {
    await page.emulateMedia({ colorScheme: theme });
    await page.goto('/settings');
    await expect(page.getByText('Remote Server Connection')).toBeVisible();
    await expectResolvedTheme(page, theme);
    await expectNoCriticalOrSeriousA11yViolations(page);

    await page.goto('/remote/projects/remote-1');
    await expect(
      page.getByRole('heading', { name: 'remote-python' }),
    ).toBeVisible();
    await expect(page.getByText('Remote project details')).toBeVisible();
    await expectResolvedTheme(page, theme);
    await expectNoCriticalOrSeriousA11yViolations(page);
  });
}
