import { expect, type Page, test } from '@playwright/test';
import {
  expectNoCriticalOrSeriousA11yViolations,
  expectResolvedTheme,
  mockApi,
  selectTheme,
  signIn,
} from './helpers';

const majorPages: Array<{
  path: string;
  assertReady: (page: Page) => Promise<void>;
}> = [
  {
    path: '/workspaces',
    assertReady: async (page) => {
      await expect(
        page.getByRole('heading', { name: 'Workspaces' }),
      ).toBeVisible();
      await expect(page.getByText('analytics-workspace')).toBeVisible();
    },
  },
  {
    path: '/workspaces/ws-seed',
    assertReady: async (page) => {
      await expect(
        page.getByRole('heading', { name: 'analytics-workspace' }),
      ).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();
    },
  },
  {
    path: '/registries',
    assertReady: async (page) => {
      await expect(
        page.getByRole('heading', { name: 'Registries' }),
      ).toBeVisible();
      await expect(page.getByText('quay.io')).toBeVisible();
    },
  },
  {
    path: '/registries/reg-1',
    assertReady: async (page) => {
      await expect(page.getByRole('heading', { name: 'Quay' })).toBeVisible();
      await expect(
        page.getByRole('cell', { name: 'nebari/python', exact: true }),
      ).toBeVisible();
    },
  },
  {
    path: '/settings',
    assertReady: async (page) => {
      await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
      await expect(page.getByText('Remote Server Connection')).toBeVisible();
    },
  },
  {
    path: '/admin',
    assertReady: async (page) => {
      await expect(page.getByText('Quick Actions')).toBeVisible();
      await expect(page.getByText('Total Users')).toBeVisible();
    },
  },
  {
    path: '/admin/users',
    assertReady: async (page) => {
      await expect(
        page.getByRole('heading', { name: 'User Management' }),
      ).toBeVisible();
      await expect(page.getByText('viewer@example.com')).toBeVisible();
    },
  },
  {
    path: '/admin/groups',
    assertReady: async (page) => {
      await expect(page.getByRole('heading', { name: 'Groups' })).toBeVisible();
      await expect(page.getByText('data-science')).toBeVisible();
    },
  },
  {
    path: '/admin/registries',
    assertReady: async (page) => {
      await expect(
        page.getByRole('heading', { name: 'OCI Registry Management' }),
      ).toBeVisible();
      await expect(page.getByText('robot')).toBeVisible();
    },
  },
  {
    path: '/admin/audit-logs',
    assertReady: async (page) => {
      await expect(
        page.getByRole('heading', { name: 'Audit Logs' }),
      ).toBeVisible();
      await expect(
        page.getByRole('cell', { name: 'create user', exact: true }),
      ).toBeVisible();
    },
  },
  {
    path: '/remote/workspaces/remote-1',
    assertReady: async (page) => {
      await expect(
        page.getByRole('heading', { name: 'remote-python' }),
      ).toBeVisible();
      await expect(page.getByText('Remote workspace details')).toBeVisible();
    },
  },
];

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

test('honors system dark mode and theme menu choices with a11y checks @a11y', async ({
  page,
}) => {
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.goto('/login');

  await expectResolvedTheme(page, 'dark');
  await expect(page.getByAltText('Nebi Logo')).toHaveAttribute(
    'src',
    /nebi-logo-dark.svg$/,
  );
  await expectNoCriticalOrSeriousA11yViolations(page);

  await signIn(page);

  await page.getByRole('button', { name: /testuser/i }).click();
  await expect(
    page.getByRole('menuitemradio', { name: 'System theme' }),
  ).toHaveAttribute('aria-checked', 'true');

  await page.getByRole('menuitemradio', { name: 'Light mode' }).click();
  await expectResolvedTheme(page, 'light');
  await expect(
    page.getByRole('menuitemradio', { name: 'Light mode' }),
  ).toHaveAttribute('aria-checked', 'true');

  await page.getByRole('menuitemradio', { name: 'Dark mode' }).click();
  await expectResolvedTheme(page, 'dark');
  await expect(
    page.getByRole('menuitemradio', { name: 'Dark mode' }),
  ).toHaveAttribute('aria-checked', 'true');
  await expectNoCriticalOrSeriousA11yViolations(page);
});

for (const theme of ['light', 'dark'] as const) {
  test(`major pages pass critical a11y checks in ${theme} mode @a11y`, async ({
    page,
  }) => {
    test.slow();
    await signIn(page);
    await selectTheme(page, theme);

    for (const { path, assertReady } of majorPages) {
      await page.goto(path);
      await assertReady(page);
      await expectResolvedTheme(page, theme);
      await expectNoCriticalOrSeriousA11yViolations(page);
    }
  });
}
