import type { Page } from '@playwright/test';

const BASE = '/api/v1';

const mockUser = {
  id: 'user-1',
  username: 'testuser',
  email: 'test@example.com',
  is_admin: false,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

const mockAdminUser = {
  ...mockUser,
  id: 'admin-1',
  username: 'admin',
  email: 'admin@example.com',
  is_admin: true,
};

const mockWorkspace = {
  id: 'ws-1',
  name: 'test-workspace',
  owner_id: 'user-1',
  owner: { id: 'user-1', username: 'testuser', email: 'test@example.com' },
  status: 'ready',
  package_manager: 'pixi',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  size_bytes: 1024,
  size_formatted: '1 KB',
};

const mockJob = {
  id: 'job-1',
  workspace_id: 'ws-1',
  type: 'create',
  status: 'completed',
  logs: 'Job completed successfully',
  created_at: '2024-01-01T00:00:00Z',
};

const mockRegistry = {
  id: 'reg-1',
  name: 'My Registry',
  url: 'https://registry.example.com',
  username: 'reguser',
  has_api_token: false,
  is_default: true,
  namespace: 'myorg',
  created_at: '2024-01-01T00:00:00Z',
};

const mockPublication = {
  id: 'pub-1',
  registry_name: 'My Registry',
  registry_url: 'https://registry.example.com',
  registry_namespace: 'myorg',
  repository: 'test-workspace',
  tag: 'v1.0.0',
  digest: 'sha256:abc123',
  is_public: true,
  published_by: 'user-1',
  published_at: '2024-01-01T00:00:00Z',
};

async function setupCommonRoutes(page: Page) {
  await page.route(`**${BASE}/version`, (route) =>
    route.fulfill({ json: { mode: 'team', features: {}, version: '0.0.1' } })
  );

  await page.route(`**${BASE}/auth/session`, (route) =>
    route.fulfill({ status: 401, json: { message: 'no session' } })
  );

  await page.route(`**${BASE}/auth/me`, (route) =>
    route.fulfill({ json: mockUser })
  );

  await page.route(`**${BASE}/auth/login`, (route) =>
    route.fulfill({ json: { token: 'test-token', user: mockUser } })
  );

  await page.route(`**${BASE}/workspaces`, async (route) => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON();
      return route.fulfill({ status: 201, json: { ...mockWorkspace, name: body.name } });
    }
    return route.fulfill({ json: [mockWorkspace] });
  });

  await page.route(`**${BASE}/workspaces/ws-1`, async (route) => {
    if (route.request().method() === 'DELETE') {
      return route.fulfill({ status: 204 });
    }
    return route.fulfill({ json: mockWorkspace });
  });

  await page.route(`**${BASE}/workspaces/ws-1/pixi-toml`, async (route) => {
    if (route.request().method() === 'PUT') {
      return route.fulfill({ status: 204 });
    }
    return route.fulfill({ json: { content: '[workspace]\nname = "test-workspace"\n' } });
  });

  await page.route(`**${BASE}/workspaces/ws-1/packages`, async (route) => {
    if (route.request().method() === 'POST') {
      return route.fulfill({ status: 204 });
    }
    return route.fulfill({ json: [] });
  });

  await page.route(`**${BASE}/workspaces/ws-1/packages/**`, (route) =>
    route.fulfill({ status: 204 })
  );

  await page.route(`**${BASE}/workspaces/ws-1/versions`, (route) =>
    route.fulfill({ json: [] })
  );

  await page.route(`**${BASE}/workspaces/ws-1/tags`, (route) =>
    route.fulfill({ json: [] })
  );

  await page.route(`**${BASE}/workspaces/ws-1/collaborators`, (route) =>
    route.fulfill({ json: [] })
  );

  await page.route(`**${BASE}/workspaces/ws-1/publications`, (route) =>
    route.fulfill({ json: [mockPublication] })
  );

  await page.route(`**${BASE}/workspaces/ws-1/publish-defaults`, (route) =>
    route.fulfill({
      json: {
        registry_id: 'reg-1',
        registry_name: 'My Registry',
        namespace: 'myorg',
        repository: 'test-workspace',
        tag: 'latest',
      },
    })
  );

  await page.route(`**${BASE}/jobs`, (route) =>
    route.fulfill({ json: [mockJob] })
  );

  await page.route(`**${BASE}/jobs/**`, (route) =>
    route.fulfill({ json: mockJob })
  );

  await page.route(`**${BASE}/registries`, (route) =>
    route.fulfill({ json: [mockRegistry] })
  );

  await page.route(`**${BASE}/registries/reg-1/repositories`, (route) =>
    route.fulfill({
      json: {
        repositories: [{ name: 'myorg/test-workspace', is_public: true }],
        fallback: false,
      },
    })
  );

  await page.route(`**${BASE}/registries/reg-1/tags*`, (route) =>
    route.fulfill({ json: { tags: [{ name: 'v1.0.0' }, { name: 'latest' }] } })
  );

  await page.route(`**${BASE}/registries/reg-1/import`, (route) =>
    route.fulfill({ status: 201, json: { ...mockWorkspace, id: 'ws-imported' } })
  );

  // Admin — non-admin user gets 403 on admin endpoints
  await page.route(`**${BASE}/admin/**`, (route) =>
    route.fulfill({ status: 403, json: { error: 'Forbidden' } })
  );
}

async function setupAdminRoutes(page: Page) {
  await page.route(`**${BASE}/admin/users`, async (route) => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON();
      return route.fulfill({
        status: 201,
        json: { ...mockUser, id: 'user-new', username: body.username, email: body.email },
      });
    }
    return route.fulfill({ json: [mockUser, mockAdminUser] });
  });

  await page.route(`**${BASE}/admin/users/*/toggle-admin`, (route) =>
    route.fulfill({ status: 204 })
  );

  await page.route(`**${BASE}/admin/users/*`, (route) =>
    route.fulfill({ status: 204 })
  );

  await page.route(`**${BASE}/admin/audit-logs`, (route) =>
    route.fulfill({ json: [] })
  );

  await page.route(`**${BASE}/admin/dashboard/stats`, (route) =>
    route.fulfill({
      json: { total_disk_usage_bytes: 1024, total_disk_usage_formatted: '1 KB' },
    })
  );

  await page.route(`**${BASE}/admin/registries`, async (route) => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON();
      return route.fulfill({ status: 201, json: { ...mockRegistry, id: 'reg-new', name: body.name } });
    }
    return route.fulfill({ json: [mockRegistry] });
  });

  await page.route(`**${BASE}/admin/registries/*`, async (route) => {
    if (route.request().method() === 'PUT') {
      return route.fulfill({ json: mockRegistry });
    }
    if (route.request().method() === 'DELETE') {
      return route.fulfill({ status: 204 });
    }
    return route.fulfill({ json: mockRegistry });
  });
}

export async function setupApiMocks(page: Page) {
  await setupCommonRoutes(page);
}

export async function setupApiMocksAsAdmin(page: Page) {
  await setupCommonRoutes(page);
  await setupAdminRoutes(page);
  // Override auth endpoints to use admin user
  await page.route(`**${BASE}/auth/me`, (route) =>
    route.fulfill({ json: mockAdminUser })
  );
  await page.route(`**${BASE}/auth/login`, (route) =>
    route.fulfill({ json: { token: 'admin-token', user: mockAdminUser } })
  );
}

export { mockUser, mockAdminUser, mockWorkspace, mockJob, mockRegistry };
