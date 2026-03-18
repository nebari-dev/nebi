import { test, expect } from '@playwright/test';
import { setupApiMocksAsAdmin, setupApiMocks } from './helpers/api-mocks';
import { checkA11y } from './helpers/a11y';

test.beforeEach(async ({ page }) => {
  await setupApiMocksAsAdmin(page);
});

test('admin dashboard shows all stat cards', async ({ page }) => {
  await page.goto('/admin');
  await expect(page.getByText('Total Users')).toBeVisible();
  await expect(page.getByText('Environments')).toBeVisible();
  await expect(page.getByText('Active Jobs')).toBeVisible();
  await expect(page.getByText('Disk Usage')).toBeVisible();
});

test('admin dashboard quick action links are present', async ({ page }) => {
  await page.goto('/admin');
  await expect(page.getByText('Manage Users')).toBeVisible();
  await expect(page.getByText('Manage Registries')).toBeVisible();
  await expect(page.getByText('View Audit Logs')).toBeVisible();
});

test('users page lists users', async ({ page }) => {
  await page.goto('/admin/users');
  await expect(page.getByText('testuser')).toBeVisible();
  await expect(page.getByText('admin')).toBeVisible();
});

test('create user dialog opens and submits', async ({ page }) => {
  await page.goto('/admin/users');
  await page.getByRole('button', { name: /create user/i }).click();
  await expect(page.getByRole('dialog')).toBeVisible();

  await page.getByLabel(/username/i).fill('newuser');
  await page.getByLabel(/email/i).fill('new@example.com');
  await page.getByLabel(/password/i).fill('password123');
  await page.getByRole('button', { name: /create/i }).last().click();

  await expect(page.getByRole('dialog')).not.toBeVisible();
});

test('delete user opens confirm dialog', async ({ page }) => {
  await page.goto('/admin/users');
  await page.getByRole('button', { name: /delete/i }).first().click();
  await expect(page.getByRole('alertdialog')).toBeVisible();
  await page.getByRole('button', { name: /delete/i }).last().click();
  await expect(page.getByRole('alertdialog')).not.toBeVisible();
});

test('audit logs page renders', async ({ page }) => {
  await page.goto('/admin/audit-logs');
  await expect(page.getByRole('heading', { name: /audit logs/i })).toBeVisible();
});

test('admin registries page lists registry', async ({ page }) => {
  await page.goto('/admin/registries');
  await expect(page.getByText('My Registry')).toBeVisible();
});

test('non-admin is redirected from /admin', async ({ page }) => {
  await setupApiMocks(page);
  await page.goto('/admin');
  // Admin route checks useIsAdmin — 403 response causes redirect or restricted view
  await expect(page.getByRole('heading', { name: /admin/i })).not.toBeVisible();
});

test('admin dashboard passes accessibility audit', async ({ page }) => {
  await page.goto('/admin');
  await expect(page.getByText('Total Users')).toBeVisible();
  await checkA11y(page, 'admin dashboard');
});

test('users page passes accessibility audit', async ({ page }) => {
  await page.goto('/admin/users');
  await expect(page.getByText('testuser')).toBeVisible();
  await checkA11y(page, 'admin users');
});

test('audit logs page passes accessibility audit', async ({ page }) => {
  await page.goto('/admin/audit-logs');
  await expect(page.getByRole('heading', { name: /audit logs/i })).toBeVisible();
  await checkA11y(page, 'admin audit logs');
});

test('admin registries page passes accessibility audit', async ({ page }) => {
  await page.goto('/admin/registries');
  await expect(page.getByText('My Registry')).toBeVisible();
  await checkA11y(page, 'admin registries');
});
