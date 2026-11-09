// Owner: U1 (projects). MSW handlers for /api/v1/client project routes.
import { HttpResponse, http } from 'msw';
import {
  mockCapabilities,
  mockJobSummary,
  mockProjectItem,
} from '@/test/fixtures/contracts';

const C = '/api/v1/client';

export const projectsHandlers = [
  http.get(`${C}/capabilities`, () => HttpResponse.json(mockCapabilities)),
  http.get(`${C}/projects`, () => HttpResponse.json([mockProjectItem])),
  http.get(`${C}/projects/name-check`, ({ request }) => {
    const name = new URL(request.url).searchParams.get('name') ?? '';
    return HttpResponse.json({ name, available: true });
  }),
  http.get(`${C}/projects/:id`, ({ params }) =>
    HttpResponse.json({ ...mockProjectItem, id: params.id as string }),
  ),
  http.post(`${C}/projects`, async ({ request }) => {
    const body = (await request.json()) as { name: string };
    return HttpResponse.json(
      {
        project: { ...mockProjectItem, name: body.name, status: 'pending' },
        job: { ...mockJobSummary, type: 'create', status: 'pending' },
      },
      { status: 202 },
    );
  }),
  http.delete(`${C}/projects/:id`, () =>
    HttpResponse.json(
      { ...mockJobSummary, type: 'delete', status: 'pending' },
      { status: 202 },
    ),
  ),
  http.post(`${C}/manifest/parse`, () =>
    HttpResponse.json({
      form: { name: 'Project_1', platforms: [], channels: [], requested: [] },
      unrepresentable: [],
    }),
  ),
  http.post(`${C}/manifest/render`, () =>
    HttpResponse.json({ pixi_toml: '[workspace]\nname = "Project_1"\n' }),
  ),
];
