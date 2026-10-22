import { defineConfig, devices } from '@playwright/test';

const port = Number(process.env.PLAYWRIGHT_PORT || 8461);
// Playwright starts both frontends; use the next port for the client so they
// can run together and client tests reach the correct app.
const clientPort = port + 1;
const clientBaseURL = `http://127.0.0.1:${clientPort}`;
const baseURL = `http://127.0.0.1:${port}`;

export default defineConfig({
  testDir: './e2e',
  outputDir: './test-results',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
  ],
  use: {
    baseURL,
    trace: 'on-first-retry',
  },
  webServer: [
    {
      command: `npm run dev:server -- --host 127.0.0.1 --port ${port}`,
      url: baseURL,
      reuseExistingServer: !process.env.CI,
    },
    {
      command: `npm run dev:client -- --host 127.0.0.1 --port ${clientPort}`,
      url: clientBaseURL,
      reuseExistingServer: !process.env.CI,
    },
  ],
  projects: [
    {
      name: 'chromium',
      testIgnore: '**/client.spec.ts',
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'firefox',
      testIgnore: '**/client.spec.ts',
      use: { ...devices['Desktop Firefox'] },
    },
    {
      name: 'webkit',
      testIgnore: '**/client.spec.ts',
      use: { ...devices['Desktop Safari'] },
    },
    ...[
      ['chromium', 'Desktop Chrome'],
      ['firefox', 'Desktop Firefox'],
      ['webkit', 'Desktop Safari'],
    ].map(([name, device]) => ({
      name: `client-${name}`,
      testMatch: '**/client.spec.ts',
      use: { ...devices[device], baseURL: clientBaseURL },
    })),
  ],
});
