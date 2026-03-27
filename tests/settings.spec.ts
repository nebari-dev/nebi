import { test, expect } from '@playwright/test';
import { setupApiMocks } from './helpers/api-mocks';
import { checkA11y } from './helpers/a11y';

const mockConnectedServer = {
  status: 'connected',
  url: 'https://nebi.example.com',
  username: 'remoteuser',
};

const mockDisconnectedServer = {
  status: 'disconnected',
  url: null,
  username: null,
};

async function setupSettingsMocks(page: Parameters<typeof setupApiMocks>[0], serverStatus = mockDisconnectedServer) {
  await setupApiMocks(page);
  // Override version to local mode so the Settings nav link appears and auth is bypassed
  await page.route('**/api/v1/version', (route) =>
    route.fulfill({ json: { mode: 'local', features: {}, version: '0.0.1' } })
  );
  await page.route('**/api/v1/remote/server', async (route) => {
    if (route.request().method() === 'DELETE') {
      return route.fulfill({ status: 204 });
    }
    return route.fulfill({ json: serverStatus });
  });
  await page.route('**/api/v1/remote/connect', (route) =>
    route.fulfill({ json: mockConnectedServer })
  );
}

test.beforeEach(async ({ page }) => {
  await setupSettingsMocks(page);
  await page.goto('/settings');
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
});

test('settings page renders disconnected state', async ({ page }) => {
  await expect(page.getByText('Remote Server Connection')).toBeVisible();
  await expect(page.getByText('Disconnected')).toBeVisible();
  await expect(page.getByPlaceholder('https://nebi.example.com')).toBeVisible();
  await expect(page.getByPlaceholder('Username')).toBeVisible();
  await expect(page.getByPlaceholder('Password')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Connect' })).toBeVisible();
});

test('successful connection shows connected state', async ({ page }) => {
  await page.route('**/api/v1/remote/server', async (route) => {
    if (route.request().method() === 'DELETE') {
      return route.fulfill({ status: 204 });
    }
    return route.fulfill({ json: mockConnectedServer });
  });

  await page.getByPlaceholder('https://nebi.example.com').fill('https://nebi.example.com');
  await page.getByPlaceholder('Username').fill('remoteuser');
  await page.getByPlaceholder('Password').fill('password123');
  await page.getByRole('button', { name: 'Connect' }).click();

  await expect(page.getByText('Connected')).toBeVisible();
  await expect(page.getByText('https://nebi.example.com')).toBeVisible();
  await expect(page.getByText('remoteuser')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Disconnect' })).toBeVisible();
});

test('connection failure shows error message', async ({ page }) => {
  await page.route('**/api/v1/remote/connect', (route) =>
    route.fulfill({ status: 400, json: { error: 'Unable to reach server' } })
  );

  await page.getByPlaceholder('https://nebi.example.com').fill('https://bad-server.example.com');
  await page.getByPlaceholder('Username').fill('user');
  await page.getByPlaceholder('Password').fill('wrong');
  await page.getByRole('button', { name: 'Connect' }).click();

  await expect(page.getByText('Unable to reach server')).toBeVisible();
});

test('connected state shows server info and disconnect button', async ({ page }) => {
  await setupSettingsMocks(page, mockConnectedServer);
  await page.goto('/settings');
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();

  await expect(page.getByText('Connected')).toBeVisible();
  await expect(page.getByText('https://nebi.example.com')).toBeVisible();
  await expect(page.getByText('remoteuser')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Disconnect' })).toBeVisible();
  await expect(page.getByPlaceholder('https://nebi.example.com')).not.toBeVisible();
});

test('disconnect returns to disconnected state', async ({ page }) => {
  await setupSettingsMocks(page, mockConnectedServer);
  await page.goto('/settings');
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();

  // After disconnect, mock returns disconnected state
  await page.route('**/api/v1/remote/server', (route) =>
    route.fulfill({ json: mockDisconnectedServer })
  );
  await page.getByRole('button', { name: 'Disconnect' }).click();

  await expect(page.getByText('Disconnected')).toBeVisible();
  await expect(page.getByPlaceholder('https://nebi.example.com')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Connect' })).toBeVisible();
});

test('settings page passes accessibility audit', async ({ page }) => {
  await checkA11y(page, 'settings - disconnected');
});

test('settings connected state passes accessibility audit', async ({ page }) => {
  await setupSettingsMocks(page, mockConnectedServer);
  await page.goto('/settings');
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();

  await checkA11y(page, 'settings - connected');
});
