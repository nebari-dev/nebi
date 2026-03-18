import { test as setup, expect } from '@playwright/test';
import path from 'path';
import { setupApiMocks, setupApiMocksAsAdmin } from './helpers/api-mocks';

const userAuthFile = path.join(__dirname, '../playwright/.auth/user.json');
const adminAuthFile = path.join(__dirname, '../playwright/.auth/admin.json');

setup('authenticate as regular user', async ({ page }) => {
  await setupApiMocks(page);

  await page.goto('/login');
  await page.getByPlaceholder('Username').fill('testuser');
  await page.getByPlaceholder('Password').fill('password');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await page.waitForURL('**/workspaces');

  await page.context().storageState({ path: userAuthFile });
});

setup('authenticate as admin user', async ({ page }) => {
  await setupApiMocksAsAdmin(page);

  await page.goto('/login');
  await page.getByPlaceholder('Username').fill('admin');
  await page.getByPlaceholder('Password').fill('password');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await page.waitForURL('**/workspaces');

  await page.context().storageState({ path: adminAuthFile });
});
