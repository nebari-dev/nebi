import AxeBuilder from '@axe-core/playwright';
import { expect, type Page, type Route, test } from '@playwright/test';

const mockUser = {
  id: 'user-1',
  username: 'testuser',
  email: 'test@example.com',
  is_admin: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const makeWorkspace = (id: string, name: string) => ({
  id,
  name,
  owner_id: mockUser.id,
  owner: mockUser,
  status: 'ready',
  package_manager: 'pixi',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  size_bytes: 1024,
  size_formatted: '1 KB',
});

const fulfillJson = async (route: Route, body: unknown, status = 200) => {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  });
};

const mockApi = async (page: Page) => {
  const workspaces = [makeWorkspace('ws-seed', 'analytics-workspace')];
  const jobs = [
    {
      id: 'job-1',
      workspace_id: 'ws-created',
      type: 'create',
      status: 'completed',
      logs: 'Workspace created successfully',
      created_at: '2026-01-01T00:01:00Z',
      completed_at: '2026-01-01T00:01:05Z',
    },
  ];

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, '');
    const method = request.method();

    if (method === 'GET' && path === '/version') {
      await fulfillJson(route, {
        mode: 'team',
        features: {},
        version: '0.0.0-test',
        commit: 'e2e',
      });
      return;
    }

    if (method === 'GET' && path === '/remote/server') {
      await fulfillJson(route, { status: 'disconnected' });
      return;
    }

    if (method === 'GET' && path === '/admin/users') {
      await fulfillJson(route, { error: 'forbidden' }, 403);
      return;
    }

    if (method === 'POST' && path === '/auth/login') {
      await fulfillJson(route, { token: 'test-token', user: mockUser });
      return;
    }

    if (method === 'GET' && path === '/workspaces') {
      await fulfillJson(route, workspaces);
      return;
    }

    if (method === 'POST' && path === '/workspaces') {
      const body = request.postDataJSON() as { name?: string };
      const workspace = makeWorkspace(
        'ws-created',
        body.name || 'created-workspace',
      );
      workspaces.push(workspace);
      await fulfillJson(route, workspace, 201);
      return;
    }

    const workspacePackagesMatch = path.match(
      /^\/workspaces\/([^/]+)\/packages$/,
    );
    if (method === 'GET' && workspacePackagesMatch) {
      await fulfillJson(route, []);
      return;
    }

    const workspaceCollaboratorsMatch = path.match(
      /^\/workspaces\/([^/]+)\/collaborators$/,
    );
    if (method === 'GET' && workspaceCollaboratorsMatch) {
      await fulfillJson(route, [
        {
          kind: 'user',
          user_id: mockUser.id,
          username: mockUser.username,
          email: mockUser.email,
          role: 'owner',
          is_owner: true,
        },
      ]);
      return;
    }

    const workspacePublicationsMatch = path.match(
      /^\/workspaces\/([^/]+)\/publications$/,
    );
    if (method === 'GET' && workspacePublicationsMatch) {
      await fulfillJson(route, []);
      return;
    }

    const workspaceMatch = path.match(/^\/workspaces\/([^/]+)$/);
    if (method === 'GET' && workspaceMatch) {
      const workspace = workspaces.find(
        (item) => item.id === workspaceMatch[1],
      );
      await fulfillJson(
        route,
        workspace || { error: 'not found' },
        workspace ? 200 : 404,
      );
      return;
    }

    if (method === 'GET' && path === '/jobs') {
      await fulfillJson(route, jobs);
      return;
    }

    await fulfillJson(route, { error: `Unhandled ${method} ${path}` }, 404);
  });
};

const expectNoCriticalOrSeriousA11yViolations = async (page: Page) => {
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
    .analyze();
  const violations = results.violations
    .filter(
      (violation) =>
        violation.impact === 'critical' || violation.impact === 'serious',
    )
    .map(({ id, impact, description, nodes }) => ({
      id,
      impact,
      description,
      targets: nodes.map((node) => node.target).slice(0, 3),
    }));

  expect(violations).toEqual([]);
};

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
  await page.getByLabel('Workspace Name').fill('e2e-created-workspace');
  await page.getByRole('button', { name: 'Create & Save' }).click();

  await expect(page).toHaveURL(/\/workspaces\/ws-created$/);
  await expect(
    page.getByRole('heading', { name: 'e2e-created-workspace' }),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Jobs' }).click();
  await expect(page.getByRole('heading', { name: 'Jobs' })).toBeVisible();
  await expect(page.getByText('Workspace created successfully')).toBeVisible();
  await expectNoCriticalOrSeriousA11yViolations(page);
});
