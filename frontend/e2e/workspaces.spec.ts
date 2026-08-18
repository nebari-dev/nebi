import { expect, test } from '@playwright/test';
import { expectNoCriticalOrSeriousA11yViolations, mockApi } from './helpers';

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

test('signs in, creates a workspace, and passes critical a11y checks @a11y', async ({
  page,
}) => {
  await page.goto('/login');

  await page.getByPlaceholder('Username').fill('testuser');
  await page.getByPlaceholder('Password').fill('password123');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();

  await expect(page.getByRole('heading', { name: 'Workspaces' })).toBeVisible();
  await expect(page.getByText('analytics-workspace')).toBeVisible();
  await expectNoCriticalOrSeriousA11yViolations(page);

  await page.getByRole('button', { name: 'New Workspace' }).click();
  await page
    .getByLabel('pixi.toml Configuration')
    .fill(`[workspace]
name = "e2e-created-workspace"
channels = ["conda-forge"]
platforms = ["osx-arm64", "linux-64"]

[dependencies]
python = ">=3.11"
`);
  await page.getByRole('button', { name: 'Create & Save' }).click();

  await expect(page).toHaveURL(/\/workspaces\/ws-created$/);
  await expect(
    page.getByRole('heading', { name: 'e2e-created-workspace' }),
  ).toBeVisible();
  await page.getByRole('tab', { name: 'Jobs' }).click();
  await expect(page.getByRole('heading', { name: 'Jobs' })).toBeVisible();
  await expect(page.getByText('Workspace created successfully')).toBeVisible();
  await expectNoCriticalOrSeriousA11yViolations(page);
});
