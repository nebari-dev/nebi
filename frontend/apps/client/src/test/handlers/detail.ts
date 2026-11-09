// Owner: U2 (detail).
import { HttpResponse, http } from 'msw';
import {
  mockJobSummary,
  mockVersionDetail,
  mockVersionItem,
} from '@/test/fixtures/contracts';

const C = '/api/v1/client';

export const detailHandlers = [
  http.get(`${C}/projects/:id/versions`, () =>
    HttpResponse.json([mockVersionItem]),
  ),
  http.get(`${C}/projects/:id/versions/:version`, () =>
    HttpResponse.json(mockVersionDetail),
  ),
  http.get(`${C}/projects/:id/versions/:version/files/:name`, ({ params }) =>
    HttpResponse.json({ name: params.name as string, content: '' }),
  ),
  http.get(`${C}/projects/:id/versions/:version/packages`, () =>
    HttpResponse.json({ platforms: [], packages: {}, total_size: {} }),
  ),
  http.post(`${C}/projects/:id/install`, () =>
    HttpResponse.json(
      { ...mockJobSummary, status: 'pending' },
      { status: 202 },
    ),
  ),
  http.post(`${C}/projects/:id/uninstall`, () =>
    HttpResponse.json(
      { ...mockJobSummary, type: 'uninstall', status: 'pending' },
      { status: 202 },
    ),
  ),
];
