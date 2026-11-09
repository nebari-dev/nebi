// Owner: U5 (remote). Default: not connected.
import { HttpResponse, http } from 'msw';
import {
  mockDisconnected,
  mockJobSummary,
  mockServerProject,
  PROJECT_ID,
} from '@/test/fixtures/contracts';

const C = '/api/v1/client';

export const remoteHandlers = [
  http.get(`${C}/remote/server`, () => HttpResponse.json(mockDisconnected)),
  http.get(`${C}/remote/projects`, () =>
    HttpResponse.json({ projects: [mockServerProject], last_synced_at: null }),
  ),
  http.post(`${C}/remote/projects/:serverProjectId/pull`, () =>
    HttpResponse.json(
      {
        project_id: PROJECT_ID,
        job: { ...mockJobSummary, type: 'pull_install', status: 'pending' },
      },
      { status: 202 },
    ),
  ),
];
