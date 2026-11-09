// Owner: U4 (compare).
import { HttpResponse, http } from 'msw';

const C = '/api/v1/client';

export const compareHandlers = [
  http.get(`${C}/projects/:id/compare`, ({ request }) => {
    const q = new URL(request.url).searchParams;
    const ref = (n: number) => ({
      number: n,
      server_only: false,
      installed: false,
      server_latest: false,
    });
    return HttpResponse.json({
      base: ref(Number(q.get('base'))),
      target: ref(Number(q.get('target'))),
      platform: 'linux-64',
      requested: [],
      requested_counts: { added: 0, removed: 0, changed: 0, same: 0 },
      resolved: [],
      resolved_counts: { added: 0, removed: 0, changed: 0, same: 0 },
      channels: { added: [], removed: [] },
      platforms: { added: [], removed: [] },
    });
  }),
];
