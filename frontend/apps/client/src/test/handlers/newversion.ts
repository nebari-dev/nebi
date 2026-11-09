// Owner: U3 (newversion).
import { HttpResponse, http } from 'msw';
import { mockJobSummary } from '@/test/fixtures/contracts';

const C = '/api/v1/client';

export const newVersionHandlers = [
  http.post(`${C}/projects/:id/versions`, () =>
    HttpResponse.json(
      { ...mockJobSummary, type: 'create_version', status: 'pending' },
      { status: 202 },
    ),
  ),
  http.post(`${C}/projects/:id/versions/preview`, () =>
    HttpResponse.json({
      platform: 'linux-64',
      requested: [],
      requested_counts: { added: 0, removed: 0, changed: 0, same: 0 },
      resolved: [],
      resolved_counts: { added: 0, removed: 0, changed: 0, same: 0 },
      channels: { added: [], removed: [] },
      platforms: { added: [], removed: [] },
    }),
  ),
];
