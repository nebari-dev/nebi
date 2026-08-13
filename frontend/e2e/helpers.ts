import AxeBuilder from '@axe-core/playwright';
import { expect, type Page, type Route } from '@playwright/test';

export const mockUser = {
  id: 'user-1',
  username: 'testuser',
  email: 'test@example.com',
  is_admin: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const secondaryUser = {
  id: 'user-2',
  username: 'viewer',
  email: 'viewer@example.com',
  is_admin: false,
  created_at: '2026-01-02T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
};

export const makeWorkspace = (id: string, name: string) => ({
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

const registries = [
  {
    id: 'reg-1',
    name: 'Quay',
    url: 'quay.io',
    username: 'robot',
    has_api_token: true,
    is_default: true,
    namespace: 'nebari',
    created_at: '2026-01-01T00:00:00Z',
  },
];

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

const groups = [
  {
    id: 'group-1',
    name: 'data-science',
    description: 'Data science users',
    source: 'native',
    member_count: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
];

const dashboardStats = {
  total_disk_usage_bytes: 2048,
  total_disk_usage_formatted: '2 KB',
};

const pixiToml = `[workspace]
name = "analytics-workspace"
channels = ["conda-forge"]
platforms = ["osx-arm64", "linux-64"]

[dependencies]
python = ">=3.11"
`;

export const fulfillJson = async (
  route: Route,
  body: unknown,
  status = 200,
) => {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  });
};

export const mockApi = async (page: Page) => {
  const workspaces = [makeWorkspace('ws-seed', 'analytics-workspace')];

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

    if (method === 'POST' && path === '/auth/login') {
      await fulfillJson(route, { token: 'test-token', user: mockUser });
      return;
    }

    if (method === 'GET' && path === '/remote/server') {
      await fulfillJson(route, { status: 'disconnected' });
      return;
    }

    if (method === 'GET' && path === '/admin/users') {
      await fulfillJson(route, [mockUser, secondaryUser]);
      return;
    }

    if (method === 'GET' && path.match(/^\/admin\/users\/[^/]+\/groups$/)) {
      await fulfillJson(route, []);
      return;
    }

    if (method === 'GET' && path === '/admin/dashboard/stats') {
      await fulfillJson(route, dashboardStats);
      return;
    }

    if (method === 'GET' && path === '/admin/audit-logs') {
      await fulfillJson(route, [
        {
          id: 'audit-1',
          user_id: mockUser.id,
          user: mockUser,
          action: 'create_user',
          resource: 'users',
          resource_id: secondaryUser.id,
          details_json: { username: secondaryUser.username },
          timestamp: '2026-01-01T00:00:00Z',
        },
      ]);
      return;
    }

    if (method === 'GET' && path === '/admin/groups') {
      await fulfillJson(route, groups);
      return;
    }

    if (method === 'GET' && path === '/admin/registries') {
      await fulfillJson(route, registries);
      return;
    }

    if (method === 'GET' && path === '/registries') {
      await fulfillJson(route, registries);
      return;
    }

    const registryRepositoriesMatch = path.match(
      /^\/registries\/([^/]+)\/repositories$/,
    );
    if (method === 'GET' && registryRepositoriesMatch) {
      await fulfillJson(route, {
        fallback: false,
        repositories: [{ name: 'nebari/python', is_public: true }],
      });
      return;
    }

    const registryTagsMatch = path.match(/^\/registries\/([^/]+)\/tags$/);
    if (method === 'GET' && registryTagsMatch) {
      await fulfillJson(route, { tags: [{ name: 'latest' }] });
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
      await fulfillJson(route, [
        {
          id: 'package-1',
          workspace_id: workspacePackagesMatch[1],
          name: 'python',
          version: '3.11.0',
          installed_at: '2026-01-01T00:00:00Z',
        },
      ]);
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
        {
          kind: 'group',
          group_id: 'group-1',
          name: 'data-science',
          source: 'native',
          role: 'viewer',
          is_owner: false,
        },
      ]);
      return;
    }

    const workspacePublicationsMatch = path.match(
      /^\/workspaces\/([^/]+)\/publications$/,
    );
    if (method === 'GET' && workspacePublicationsMatch) {
      await fulfillJson(route, [
        {
          id: 'publication-1',
          registry_name: 'Quay',
          registry_url: 'quay.io',
          registry_namespace: 'nebari',
          repository: 'analytics-workspace',
          tag: 'latest',
          digest: 'sha256:test',
          is_public: true,
          published_by: mockUser.id,
          published_at: '2026-01-01T00:00:00Z',
        },
      ]);
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

    const remoteWorkspaceMatch = path.match(/^\/remote\/workspaces\/([^/]+)$/);
    if (method === 'GET' && remoteWorkspaceMatch) {
      await fulfillJson(route, {
        ...makeWorkspace(remoteWorkspaceMatch[1], 'remote-python'),
        size_bytes: 2048,
      });
      return;
    }

    await fulfillJson(route, { error: `Unhandled ${method} ${path}` }, 404);
  });
};

export const signIn = async (page: Page) => {
  await page.goto('/login');
  await page.getByPlaceholder('Username').fill(mockUser.username);
  await page.getByPlaceholder('Password').fill('password123');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Workspaces' })).toBeVisible();
};

export const selectTheme = async (
  page: Page,
  theme: 'light' | 'dark' | 'system',
) => {
  const labelByTheme = {
    light: 'Light mode',
    dark: 'Dark mode',
    system: 'System theme',
  };

  await page.getByRole('button', { name: /testuser/i }).click();
  await page
    .getByRole('menuitemradio', { name: labelByTheme[theme] })
    .click();
  await page.keyboard.press('Escape');
};

export const expectResolvedTheme = async (
  page: Page,
  theme: 'light' | 'dark',
) => {
  if (theme === 'dark') {
    await expect(page.locator('html')).toHaveClass(/dark/);
    return;
  }

  await expect(page.locator('html')).not.toHaveClass(/dark/);
};

export const expectNoCriticalOrSeriousA11yViolations = async (page: Page) => {
  // Disable CSS transitions/animations so axe measures settled colors rather
  // than intermediate values mid-transition (e.g. right after a theme switch),
  // which otherwise produces flaky color-contrast violations. Zeroing the
  // duration is not enough: a transition already in flight keeps running with
  // its original duration, and WebKit can leave it stuck at an intermediate
  // color for seconds under CI load. Removing the transition property cancels
  // in-flight transitions and snaps computed colors to their final values.
  await page.addStyleTag({
    content: `*, *::before, *::after {
      transition: none !important;
      animation: none !important;
    }`,
  });

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
