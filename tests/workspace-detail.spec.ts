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
  await expect(page.getByText('Ready').first()).toBeVisible();
});

test('all tabs are visible', async ({ page }) => {
  await expect(page.getByRole('tab', { name: 'Overview' })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Packages' })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Configuration' })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Versions' })).toBeVisible();
  await expect(page.getByRole('tab', { name: /Publications/ })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Jobs' })).toBeVisible();
  await expect(page.getByRole('tab', { name: /Collaborators/ })).toBeVisible();
});

test('overview tab shows workspace metadata', async ({ page }) => {
  await expect(page.getByText('Workspace Name')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'test-workspace' })).toBeVisible();
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
  await expect(page.getByText('Completed', { exact: true }).first()).toBeVisible();
});

test('configuration tab shows pixi.toml content', async ({ page }) => {
  await page.getByRole('tab', { name: 'Configuration' }).click();
  const tabPanel = page.getByRole('tabpanel');
  await expect(tabPanel.getByText('[workspace]')).toBeVisible();
  await expect(tabPanel.getByRole('button', { name: 'Copy' })).toBeVisible();
  await expect(tabPanel.getByRole('button', { name: 'Edit' })).toBeVisible();
});

test('versions tab shows empty state when no versions exist', async ({ page }) => {
  await page.getByRole('tab', { name: 'Versions' }).click();
  await expect(page.getByText('No Version History Yet')).toBeVisible();
});

test('publications tab shows publication entry', async ({ page }) => {
  await page.getByRole('tab', { name: /Publications/ }).click();
  await expect(page.getByRole('heading', { name: 'Publications' })).toBeVisible();
  await expect(page.getByText('test-workspace:v1.0.0')).toBeVisible();
  await expect(page.getByText('My Registry')).toBeVisible();
  await expect(page.getByText('Public', { exact: true })).toBeVisible();
});

test('collaborators tab shows empty state when no collaborators', async ({ page }) => {
  await page.getByRole('tab', { name: /Collaborators/ }).click();
  await expect(page.getByRole('heading', { name: 'Collaborators' })).toBeVisible();
  await expect(page.getByText('No collaborators yet')).toBeVisible();
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

test('workspace detail configuration tab passes accessibility audit', async ({ page }) => {
  await page.getByRole('tab', { name: 'Configuration' }).click();
  await expect(page.getByText('[workspace]')).toBeVisible();
  await checkA11y(page, 'workspace detail - configuration');
});

test('workspace detail versions tab passes accessibility audit', async ({ page }) => {
  await page.getByRole('tab', { name: 'Versions' }).click();
  await expect(page.getByText('No Version History Yet')).toBeVisible();
  await checkA11y(page, 'workspace detail - versions');
});

test('workspace detail publications tab passes accessibility audit', async ({ page }) => {
  await page.getByRole('tab', { name: /Publications/ }).click();
  await expect(page.getByText('test-workspace:v1.0.0')).toBeVisible();
  await checkA11y(page, 'workspace detail - publications');
});

test('workspace detail collaborators tab passes accessibility audit', async ({ page }) => {
  await page.getByRole('tab', { name: /Collaborators/ }).click();
  await expect(page.getByText('No collaborators yet')).toBeVisible();
  await checkA11y(page, 'workspace detail - collaborators');
});
