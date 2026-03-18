import { test, expect } from '@playwright/test';
import { setupApiMocks } from './helpers/api-mocks';
import { checkA11y } from './helpers/a11y';

test.beforeEach(async ({ page }) => {
  await setupApiMocks(page);
});

test('registries list loads and shows registry row', async ({ page }) => {
  await page.goto('/registries');
  await expect(page.getByRole('heading', { name: 'Registries' })).toBeVisible();
  await expect(page.getByText('My Registry')).toBeVisible();
  await expect(page.getByText('https://registry.example.com')).toBeVisible();
});

test('shows empty state when no registries', async ({ page }) => {
  await page.route('**/api/v1/registries', (route) =>
    route.fulfill({ json: [] })
  );
  await page.goto('/registries');
  await expect(page.getByText('No registries configured')).toBeVisible();
});

test('Browse navigates to registry repositories page', async ({ page }) => {
  await page.goto('/registries');
  await page.getByRole('button', { name: 'Browse' }).click();
  await page.waitForURL('**/registries/reg-1');
  await expect(page.getByRole('heading', { name: 'My Registry' })).toBeVisible();
});

test('repositories page shows repo list and search input', async ({ page }) => {
  await page.goto('/registries/reg-1');
  await expect(page.getByPlaceholder('Search repositories...')).toBeVisible();
  await expect(page.getByText('myorg/test-workspace')).toBeVisible();
});

test('View Tags navigates to tags page', async ({ page }) => {
  await page.goto('/registries/reg-1');
  await page.getByRole('button', { name: 'View Tags' }).click();
  await page.waitForURL('**/registries/reg-1/repo/**');
  await expect(page.getByText('v1.0.0')).toBeVisible();
  await expect(page.getByText('latest')).toBeVisible();
});

test('tags page Import button opens import form', async ({ page }) => {
  await page.goto('/registries/reg-1/repo/myorg/test-workspace');
  await page.getByRole('button', { name: 'Import' }).first().click();
  await expect(page.getByRole('heading', { name: 'Import Environment' })).toBeVisible();
  await expect(page.getByPlaceholder('Enter workspace name')).toBeVisible();
});

test('registries list page passes accessibility audit', async ({ page }) => {
  await page.goto('/registries');
  await expect(page.getByText('My Registry')).toBeVisible();
  await checkA11y(page, 'registries list');
});

test('registry repositories page passes accessibility audit', async ({ page }) => {
  await page.goto('/registries/reg-1');
  await expect(page.getByText('myorg/test-workspace')).toBeVisible();
  await checkA11y(page, 'registry repositories');
});

test('registry tags page passes accessibility audit', async ({ page }) => {
  await page.goto('/registries/reg-1/repo/myorg/test-workspace');
  await expect(page.getByText('v1.0.0')).toBeVisible();
  await checkA11y(page, 'registry tags');
});
