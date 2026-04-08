import { test, expect } from '@playwright/test';
import { setupApiMocks } from './helpers/api-mocks';
import { checkA11y } from './helpers/a11y';

// Auth tests use no storageState — override the project default
test.use({ storageState: { cookies: [], origins: [] } });

test.beforeEach(async ({ page }) => {
  await setupApiMocks(page);
});

test('login page renders expected elements', async ({ page }) => {
  await page.goto('/login');
  await expect(page.getByPlaceholder('Username')).toBeVisible();
  await expect(page.getByPlaceholder('Password')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Sign in', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Sign in with OAuth', exact: true })).toBeVisible();
});

test('shows error on bad credentials', async ({ page }) => {
  await page.route('**/api/v1/auth/login', (route) =>
    route.fulfill({ status: 401, json: { error: 'Invalid credentials' } })
  );
  await page.goto('/login');
  await page.getByPlaceholder('Username').fill('wrong');
  await page.getByPlaceholder('Password').fill('wrong');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.getByText('Invalid credentials')).toBeVisible();
});

test('successful login redirects to workspaces', async ({ page }) => {
  await page.goto('/login');
  await page.getByPlaceholder('Username').fill('testuser');
  await page.getByPlaceholder('Password').fill('password');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await page.waitForURL('**/workspaces');
  await expect(page.getByRole('heading', { name: 'Workspaces' })).toBeVisible();
});

test('unauthenticated access to protected route redirects to login', async ({ page }) => {
  await page.goto('/workspaces');
  await page.waitForURL('**/login');
  await expect(page.getByPlaceholder('Username')).toBeVisible();
});

test('login page passes accessibility audit', async ({ page }) => {
  await page.goto('/login');
  await expect(page.getByPlaceholder('Username')).toBeVisible();
  await checkA11y(page, 'login page');
});
