import { test, expect } from '@playwright/test';
import { setupApiMocks } from './helpers/api-mocks';
import { checkA11y } from './helpers/a11y';

test.beforeEach(async ({ page }) => {
  await setupApiMocks(page);
  await page.goto('/workspaces/ws-1');
  await expect(page.getByRole('heading', { name: 'test-workspace' })).toBeVisible();
});

test('renders workspace name and status badge', async ({ page }) => {
  await expect(page.getByRole('heading', { name: 'test-workspace' })).toBeVisible();
  await expect(page.getByText('Ready')).toBeVisible();
});

test('all tabs are visible', async ({ page }) => {
  await expect(page.getByRole('tab', { name: 'Overview' })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Packages' })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'pixi.toml' })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Version History' })).toBeVisible();
  await expect(page.getByRole('tab', { name: /Publications/ })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Jobs' })).toBeVisible();
});

test('overview tab shows workspace metadata', async ({ page }) => {
  await expect(page.getByText('Workspace Name')).toBeVisible();
  await expect(page.getByText('test-workspace')).toBeVisible();
  await expect(page.getByText('Package Manager')).toBeVisible();
  await expect(page.getByText('pixi')).toBeVisible();
  await expect(page.getByText('Size')).toBeVisible();
  await expect(page.getByText('1 KB')).toBeVisible();
});

test('packages tab shows empty state and disabled install button when not ready', async ({ page }) => {
  await page.route('**/api/v1/workspaces/ws-1', (route) =>
    route.fulfill({ json: { id: 'ws-1', name: 'test-workspace', status: 'creating', package_manager: 'pixi', owner_id: 'user-1', created_at: '2024-01-01T00:00:00Z', updated_at: '2024-01-01T00:00:00Z' } })
  );
  await page.reload();
  await page.getByRole('tab', { name: 'Packages' }).click();
  await expect(page.getByRole('button', { name: 'Install Package' })).toBeDisabled();
});

test('jobs tab shows job row with status badge', async ({ page }) => {
  await page.getByRole('tab', { name: 'Jobs' }).click();
  await expect(page.getByText('completed', { exact: false })).toBeVisible();
});

test('workspace detail overview tab passes accessibility audit', async ({ page }) => {
  await checkA11y(page, 'workspace detail - overview');
});

test('workspace detail packages tab passes accessibility audit', async ({ page }) => {
  await page.getByRole('tab', { name: 'Packages' }).click();
  await checkA11y(page, 'workspace detail - packages');
});

test('workspace detail jobs tab passes accessibility audit', async ({ page }) => {
  await page.getByRole('tab', { name: 'Jobs' }).click();
  await checkA11y(page, 'workspace detail - jobs');
});
