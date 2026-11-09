// Owner: U5 (remote). Default: not connected. Tests that need a connected
// server call server.use(...remoteConnectedHandlers).
import { HttpResponse, http } from 'msw';
import {
  mockDisconnected,
  mockJobSummary,
  mockServerConnection,
  mockServerProject,
  mockVersionItem,
  PROJECT_ID,
  SERVER_PROJECT_ID,
} from '@/test/fixtures/contracts';
import type {
  JobSummary,
  RemoteDetails,
  ServerProjectItem,
  ServerProjectList,
} from '@/types/contracts';

const C = '/api/v1/client';

/** A second server project that is already on this machine. */
export const mockPulledServerProject: ServerProjectItem = {
  id: '55555555-5555-4555-8555-555555555555',
  name: 'Project_1',
  owner: 'sam',
  namespace: 'Personal',
  latest_version: 6,
  updated_at: '2026-09-29T00:00:00Z',
  role: 'owner',
  local_project_id: PROJECT_ID,
};

export const mockServerProjectList: ServerProjectList = {
  projects: [mockServerProject, mockPulledServerProject],
  last_synced_at: new Date(Date.now() - 2 * 60_000).toISOString(),
};

export const mockRemoteDetails: RemoteDetails = {
  project: mockServerProject,
  url: `${mockServerConnection.url}/projects/${SERVER_PROJECT_ID}`,
  versions: [
    { ...mockVersionItem, number: 3, server_only: true, installed: false },
  ],
};

/** The pull_install job as returned by POST .../pull (pending). */
export const mockPullJob: JobSummary = {
  ...mockJobSummary,
  type: 'pull_install',
  type_label: 'Pull and install',
  status: 'pending',
  project_name: 'ml-baseline',
  version_number: null,
  started_at: null,
  completed_at: null,
  duration_ms: null,
};

export const remoteHandlers = [
  http.get(`${C}/remote/server`, () => HttpResponse.json(mockDisconnected)),
  http.put(`${C}/remote/server`, () => HttpResponse.json(mockServerConnection)),
  http.delete(
    `${C}/remote/server`,
    () => new HttpResponse(null, { status: 204 }),
  ),
  http.get(`${C}/remote/projects`, () =>
    HttpResponse.json({ projects: [mockServerProject], last_synced_at: null }),
  ),
  http.post(`${C}/remote/projects/refresh`, () =>
    HttpResponse.json({
      ...mockServerProjectList,
      last_synced_at: new Date().toISOString(),
    }),
  ),
  http.get(`${C}/remote/projects/:serverProjectId`, () =>
    HttpResponse.json(mockRemoteDetails),
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
  http.post(`${C}/projects/:id/push`, () =>
    HttpResponse.json({
      pushed_versions: [7],
      server_latest_version: 7,
      server_project_id: SERVER_PROJECT_ID,
    }),
  ),
  http.post(`${C}/projects/:id/versions/:version/pull`, ({ params }) =>
    HttpResponse.json({
      ...mockVersionItem,
      number: Number(params.version),
      server_only: false,
      installed: false,
      sync: null,
    }),
  ),
];

/** Overrides for a connected server with two server projects. */
export const remoteConnectedHandlers = [
  http.get(`${C}/remote/server`, () => HttpResponse.json(mockServerConnection)),
  http.get(`${C}/remote/projects`, () =>
    HttpResponse.json(mockServerProjectList),
  ),
];
