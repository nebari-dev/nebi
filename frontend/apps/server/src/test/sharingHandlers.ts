// In-memory team server for the Server projects page and its share dialog:
// GET /catalog/projects, DELETE /catalog/projects/:id, GET /principals and
// the /projects/:id/access routes. Tests call resetServerMock() first.
import { HttpResponse, http } from 'msw';
import type {
  AccessEntry,
  AccessList,
  CatalogProject,
  GrantAccessRequest,
  Principal,
  UpdateAccessRequest,
} from '@/types/sharing';

const API = '/api/v1';

export const mockOwnerPrincipal: Principal = {
  kind: 'user',
  id: 'user-1',
  name: 'testuser',
  email: 'test@example.com',
};
export const mockAlexPrincipal: Principal = {
  kind: 'user',
  id: 'user-alex',
  name: 'alex',
  email: 'alex@example.com',
};
export const mockJordanPrincipal: Principal = {
  kind: 'user',
  id: 'user-jordan',
  name: 'jordan',
  email: 'jordan@example.com',
};
export const mockTeamPrincipal: Principal = {
  kind: 'group',
  id: 'group-jatic',
  name: 'team-jatic',
  members: 6,
};

const directory = [
  mockOwnerPrincipal,
  mockAlexPrincipal,
  mockJordanPrincipal,
  mockTeamPrincipal,
];

export const mockCatalog = (): CatalogProject[] => [
  {
    id: 'proj-ml',
    name: 'ml-baseline',
    owner: 'testuser',
    namespace: 'Personal',
    latest_version: 4,
    updated_at: '2026-10-01T00:00:00Z',
    role: 'owner',
  },
  {
    id: 'proj-cosmos',
    name: 'jatic-cosmos',
    owner: 'someone',
    namespace: 'team-jatic',
    latest_version: 2,
    updated_at: '2026-10-02T00:00:00Z',
    role: 'view',
  },
  {
    id: 'proj-chat',
    name: 'nebi-chat',
    owner: 'testuser',
    namespace: 'Personal',
    latest_version: 7,
    updated_at: '2026-10-03T00:00:00Z',
    role: 'owner',
  },
];

const initialAccess = (p: CatalogProject): AccessList => ({
  entries: [
    {
      principal: p.role === 'owner' ? mockOwnerPrincipal : mockAlexPrincipal,
      role: 'owner',
    },
    { principal: mockTeamPrincipal, role: 'edit' },
  ],
  can_manage: p.role === 'owner',
});

let catalog: CatalogProject[] = mockCatalog();
let access: Record<string, AccessList> = {};
export let deletedProjects: string[] = [];

export const resetServerMock = (projects: CatalogProject[] = mockCatalog()) => {
  catalog = structuredClone(projects);
  access = Object.fromEntries(catalog.map((p) => [p.id, initialAccess(p)]));
  deletedProjects = [];
};
resetServerMock();

export const accessOf = (projectId: string) => access[projectId];

const sortEntries = (entries: AccessEntry[]) => {
  const [owner, ...rest] = entries;
  rest.sort((a, b) => a.principal.name.localeCompare(b.principal.name));
  return [owner, ...rest];
};

const forbidden = () =>
  HttpResponse.json(
    {
      error: 'Only the project owner or an admin can manage access',
      code: 'forbidden',
    },
    { status: 403 },
  );

export const sharingHandlers = [
  http.get(`${API}/catalog/projects`, () => HttpResponse.json(catalog)),
  http.delete(`${API}/catalog/projects/:id`, ({ params }) => {
    const p = catalog.find((x) => x.id === params.id);
    if (!p) return HttpResponse.json({ error: 'not found' }, { status: 404 });
    if (p.role !== 'owner') return forbidden();
    deletedProjects.push(p.id);
    catalog = catalog.filter((x) => x.id !== p.id);
    return new HttpResponse(null, { status: 202 });
  }),
  http.get(`${API}/principals`, ({ request }) => {
    const q = (new URL(request.url).searchParams.get('q') ?? '')
      .trim()
      .toLowerCase();
    return HttpResponse.json(
      q
        ? directory.filter(
            (p) =>
              p.name.includes(q) || (p.email ?? '').toLowerCase().includes(q),
          )
        : [],
    );
  }),
  http.get(`${API}/projects/:id/access`, ({ params }) => {
    const list = access[params.id as string];
    return list
      ? HttpResponse.json(list)
      : HttpResponse.json({ error: 'not found' }, { status: 404 });
  }),
  http.post(`${API}/projects/:id/access`, async ({ params, request }) => {
    const list = access[params.id as string];
    if (!list?.can_manage) return forbidden();
    const body = (await request.json()) as GrantAccessRequest;
    for (const ref of body.principals) {
      const p = directory.find((d) => d.kind === ref.kind && d.id === ref.id);
      if (!p) continue;
      const e = list.entries.find(
        (x) => x.principal.kind === ref.kind && x.principal.id === ref.id,
      );
      if (e) e.role = body.role;
      else list.entries.push({ principal: p, role: body.role });
    }
    list.entries = sortEntries(list.entries);
    return HttpResponse.json(list);
  }),
  http.put(
    `${API}/projects/:id/access/:kind/:principalId`,
    async ({ params, request }) => {
      const list = access[params.id as string];
      if (!list?.can_manage) return forbidden();
      const body = (await request.json()) as UpdateAccessRequest;
      const e = list.entries.find(
        (x) =>
          x.principal.kind === params.kind &&
          x.principal.id === params.principalId,
      );
      if (!e) return HttpResponse.json({ error: 'not found' }, { status: 404 });
      e.role = body.role;
      return HttpResponse.json(list);
    },
  ),
  http.delete(`${API}/projects/:id/access/:kind/:principalId`, ({ params }) => {
    const list = access[params.id as string];
    if (!list?.can_manage) return forbidden();
    list.entries = list.entries.filter(
      (x) =>
        !(
          x.principal.kind === params.kind &&
          x.principal.id === params.principalId
        ),
    );
    return HttpResponse.json(list);
  }),
];
