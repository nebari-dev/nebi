// Owner: U6 (sharing). A small in-memory team server behind the client's
// /client/remote/principals and /client/remote/projects/:id/access routes.
// Tests call resetSharingMock() (or server.use) for a fresh state.
import { HttpResponse, http } from 'msw';
import type {
  AccessEntry,
  AccessList,
  GrantAccessRequest,
  Principal,
  UpdateAccessRequest,
} from '@/types/contracts';

const C = '/api/v1/client';

export const mockSam: Principal = {
  kind: 'user',
  id: 'u-sam',
  name: 'Sam Garcia',
  email: 'sam@example.com',
};
export const mockAlex: Principal = {
  kind: 'user',
  id: 'u-alex',
  name: 'Alex Kim',
  email: 'alex@example.com',
};
export const mockJordan: Principal = {
  kind: 'user',
  id: 'u-jordan',
  name: 'Jordan Lee',
  email: 'jordan@example.com',
};
export const mockTeamJatic: Principal = {
  kind: 'group',
  id: 'g-jatic',
  name: 'team-jatic',
  members: 6,
};
export const mockTeamJobs: Principal = {
  kind: 'group',
  id: 'g-jobs',
  name: 'team-jobs',
  members: 4,
};

/** Everyone the mock server knows. */
export const mockDirectory: Principal[] = [
  mockSam,
  mockAlex,
  mockJordan,
  mockTeamJatic,
  mockTeamJobs,
];

/** Initial "Who has access": Sam owns, team-jatic edits, Alex views. */
export const initialSharedAccess = (): AccessList => ({
  entries: [
    { principal: mockSam, role: 'owner' },
    { principal: mockTeamJatic, role: 'edit' },
    { principal: mockAlex, role: 'view' },
  ],
  can_manage: true,
});

let state: AccessList = initialSharedAccess();

export const resetSharingMock = (list: AccessList = initialSharedAccess()) => {
  state = structuredClone(list);
};

export const sharingMockState = () => state;

const find = (kind: string, id: string) =>
  mockDirectory.find((p) => p.kind === kind && p.id === id);

const sorted = (entries: AccessEntry[]) => {
  const [owner, ...rest] = entries;
  rest.sort((a, b) => a.principal.name.localeCompare(b.principal.name));
  return [owner, ...rest];
};

export const sharingHandlers = [
  http.get(`${C}/remote/principals`, ({ request }) => {
    const q = (new URL(request.url).searchParams.get('q') ?? '')
      .trim()
      .toLowerCase();
    if (!q) return HttpResponse.json([]);
    return HttpResponse.json(
      mockDirectory.filter(
        (p) =>
          p.name.toLowerCase().includes(q) ||
          (p.email ?? '').toLowerCase().includes(q),
      ),
    );
  }),
  http.get(`${C}/remote/projects/:serverProjectId/access`, () =>
    HttpResponse.json(state),
  ),
  http.post(
    `${C}/remote/projects/:serverProjectId/access`,
    async ({ request }) => {
      const body = (await request.json()) as GrantAccessRequest;
      for (const ref of body.principals) {
        const p = find(ref.kind, ref.id);
        if (!p) {
          return HttpResponse.json(
            { error: 'User not found', code: 'server_invalid' },
            { status: 422 },
          );
        }
        const existing = state.entries.find(
          (e) => e.principal.kind === ref.kind && e.principal.id === ref.id,
        );
        if (existing) existing.role = body.role;
        else state.entries.push({ principal: p, role: body.role });
      }
      state = { ...state, entries: sorted(state.entries) };
      return HttpResponse.json(state);
    },
  ),
  http.put(
    `${C}/remote/projects/:serverProjectId/access/:kind/:principalId`,
    async ({ request, params }) => {
      const body = (await request.json()) as UpdateAccessRequest;
      const e = state.entries.find(
        (x) =>
          x.principal.kind === params.kind &&
          x.principal.id === params.principalId,
      );
      if (!e) {
        return HttpResponse.json(
          { error: 'No access to change', code: 'server_not_found' },
          { status: 404 },
        );
      }
      e.role = body.role;
      state = { ...state };
      return HttpResponse.json(state);
    },
  ),
  http.delete(
    `${C}/remote/projects/:serverProjectId/access/:kind/:principalId`,
    ({ params }) => {
      state = {
        ...state,
        entries: state.entries.filter(
          (x) =>
            !(
              x.principal.kind === params.kind &&
              x.principal.id === params.principalId
            ),
        ),
      };
      return HttpResponse.json(state);
    },
  ),
];
