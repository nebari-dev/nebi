import { http, HttpResponse } from 'msw';
import { setupServer } from 'msw/node';
import type { Workspace, Job, User, Collaborator, OCIRegistry, PublishDefaults, Publication } from '@/types';

export const mockUser: User = {
  id: 'user-1',
  username: 'testuser',
  email: 'test@example.com',
  is_admin: false,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

export const mockAdminUser: User = {
  ...mockUser,
  id: 'admin-1',
  username: 'admin',
  email: 'admin@example.com',
  is_admin: true,
};

export const mockWorkspace: Workspace = {
  id: 'ws-1',
  name: 'test-workspace',
  owner_id: 'user-1',
  status: 'ready',
  package_manager: 'pixi',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  size_bytes: 1024,
  size_formatted: '1 KB',
};

export const mockJob: Job = {
  id: 'job-1',
  workspace_id: 'ws-1',
  type: 'create',
  status: 'completed',
  logs: 'Job completed successfully',
  created_at: '2024-01-01T00:00:00Z',
};

export const mockCollaborator: Collaborator = {
  user_id: 'user-2',
  username: 'collaborator',
  email: 'collab@example.com',
  role: 'viewer',
  is_owner: false,
};

export const mockOwnerCollaborator: Collaborator = {
  user_id: 'user-1',
  username: 'testuser',
  email: 'test@example.com',
  role: 'owner',
  is_owner: true,
};

export const mockRegistry: OCIRegistry = {
  id: 'reg-1',
  name: 'My Registry',
  url: 'https://registry.example.com',
  username: 'reguser',
  has_api_token: false,
  is_default: true,
  namespace: 'myorg',
  created_at: '2024-01-01T00:00:00Z',
};

export const mockPublishDefaults: PublishDefaults = {
  registry_id: 'reg-1',
  registry_name: 'My Registry',
  namespace: 'myorg',
  repository: 'test-workspace',
  tag: 'latest',
};

export const mockPublication: Publication = {
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

const BASE = '/api/v1';

export const handlers = [
  http.get(`${BASE}/version`, () =>
    HttpResponse.json({ mode: 'team', features: {}, version: '0.0.1' })
  ),

  // Auth
  http.get(`${BASE}/auth/session`, () =>
    HttpResponse.json({ message: 'no session' }, { status: 401 })
  ),

  http.get(`${BASE}/auth/me`, () =>
    HttpResponse.json(mockUser)
  ),

  http.post(`${BASE}/auth/login`, () =>
    HttpResponse.json({ token: 'test-token', user: mockUser })
  ),

  // Workspaces
  http.get(`${BASE}/workspaces`, () =>
    HttpResponse.json([mockWorkspace])
  ),

  http.get(`${BASE}/workspaces/:id`, ({ params }) =>
    HttpResponse.json({ ...mockWorkspace, id: params.id as string })
  ),

  http.post(`${BASE}/workspaces`, async ({ request }) => {
    const body = await request.json() as Record<string, unknown>;
    return HttpResponse.json({ ...mockWorkspace, name: body.name as string }, { status: 201 });
  }),

  http.delete(`${BASE}/workspaces/:id`, () =>
    new HttpResponse(null, { status: 204 })
  ),

  http.get(`${BASE}/workspaces/:id/tags`, () =>
    HttpResponse.json([])
  ),

  // Collaborators
  http.get(`${BASE}/workspaces/:id/collaborators`, () =>
    HttpResponse.json([mockOwnerCollaborator, mockCollaborator])
  ),

  http.post(`${BASE}/workspaces/:id/share`, () =>
    new HttpResponse(null, { status: 204 })
  ),

  http.delete(`${BASE}/workspaces/:id/share/:userId`, () =>
    new HttpResponse(null, { status: 204 })
  ),

  // Publishing
  http.get(`${BASE}/workspaces/:id/publish-defaults`, () =>
    HttpResponse.json(mockPublishDefaults)
  ),

  http.get(`${BASE}/workspaces/:id/publications`, () =>
    HttpResponse.json([mockPublication])
  ),

  http.post(`${BASE}/workspaces/:id/publish`, () =>
    HttpResponse.json(mockJob, { status: 201 })
  ),

  // Jobs
  http.get(`${BASE}/jobs`, () =>
    HttpResponse.json([mockJob])
  ),

  http.get(`${BASE}/jobs/:id`, ({ params }) =>
    HttpResponse.json({ ...mockJob, id: params.id as string })
  ),

  // Admin
  http.get(`${BASE}/admin/users`, () =>
    HttpResponse.json([mockUser, mockAdminUser])
  ),

  http.get(`${BASE}/admin/audit-logs`, () =>
    HttpResponse.json({ logs: [], total: 0 })
  ),

  http.get(`${BASE}/admin/dashboard/stats`, () =>
    HttpResponse.json({ total_disk_usage_bytes: 0, total_disk_usage_formatted: '0 B' })
  ),

  // Registries
  http.get(`${BASE}/registries`, () =>
    HttpResponse.json([mockRegistry])
  ),
];

export const server = setupServer(...handlers);
