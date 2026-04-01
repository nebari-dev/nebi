import { test, expect } from '@playwright/test';
import { setupApiMocks } from './helpers/api-mocks';
import { checkA11y } from './helpers/a11y';

test.beforeEach(async ({ page }) => {
  await setupApiMocks(page);
});

test('workspaces list loads and renders workspace row', async ({ page }) => {
  await page.goto('/workspaces');
  await expect(page.getByRole('heading', { name: 'Workspaces' })).toBeVisible();
  await expect(page.getByText('test-workspace')).toBeVisible();
});

test('shows empty state when no workspaces', async ({ page }) => {
  await page.route('**/api/v1/workspaces', (route) => {
    if (route.request().method() === 'GET') return route.fulfill({ json: [] });
    return route.continue();
  });
  await page.goto('/workspaces');
  await expect(page.getByText('No workspaces yet')).toBeVisible();
});

test('create workspace form opens and submits', async ({ page }) => {
  await page.goto('/workspaces');
  await page.getByRole('button', { name: 'New Workspace' }).click();
  await expect(page.getByRole('heading', { name: 'Create New Workspace' })).toBeVisible();

  await page.getByPlaceholder('e.g., my-data-project').fill('my-new-workspace');
  await page.getByRole('button', { name: 'Create Workspace' }).click();

  await page.waitForURL('**/workspaces/ws-1');
});

test('delete workspace opens confirm dialog and deletes on confirm', async ({ page }) => {
  await page.goto('/workspaces');
  await page.getByRole('button', { name: /delete/i }).first().click();
  await expect(page.getByRole('heading', { name: 'Delete Workspace' })).toBeVisible();

  await page.getByRole('button', { name: 'Delete' }).click();
  await expect(page.getByRole('heading', { name: 'Delete Workspace' })).not.toBeVisible();
});

test('workspaces page passes accessibility audit', async ({ page }) => {
  await page.goto('/workspaces');
  await expect(page.getByText('test-workspace')).toBeVisible();
  await checkA11y(page, 'workspaces list');
});
