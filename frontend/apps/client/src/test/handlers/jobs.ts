// Owner: U8 (jobs). MSW handlers for /api/v1/client/jobs. The list honours
// the status / type / project_id / sort / limit query like the backend.
import { HttpResponse, http } from 'msw';
import {
  JOB_ID,
  mockJobDetail,
  mockJobSummary,
} from '@/test/fixtures/contracts';
import type { JobDetail, JobSummary } from '@/types/contracts';

const C = '/api/v1/client';

export const RUNNING_JOB_ID = '33333333-3333-4333-8333-333333333333';
export const FAILED_JOB_ID = '44444444-4444-4444-8444-444444444444';
export const QUEUED_JOB_ID = '55555555-5555-4555-8555-555555555555';
export const GONE_PROJECT_ID = '66666666-6666-4666-8666-666666666666';

const minutesAgo = (m: number) =>
  new Date(Date.now() - m * 60_000).toISOString();

/** A job list like the prototype's: one of each status. */
export function mockJobList(): JobDetail[] {
  return [
    {
      ...mockJobSummary,
      id: QUEUED_JOB_ID,
      type: 'uninstall',
      type_label: 'Uninstall',
      status: 'pending',
      version_number: null,
      created_at: minutesAgo(0),
      started_at: null,
      completed_at: null,
      duration_ms: null,
      logs: '',
    },
    {
      ...mockJobSummary,
      id: RUNNING_JOB_ID,
      type: 'create_version',
      type_label: 'Create version',
      status: 'running',
      version_number: null,
      created_at: minutesAgo(3),
      started_at: minutesAgo(2.5),
      completed_at: null,
      duration_ms: null,
      logs: 'Solving environment\n',
    },
    {
      ...mockJobSummary,
      id: FAILED_JOB_ID,
      type: 'install_version',
      type_label: 'Install version',
      status: 'failed',
      project_id: GONE_PROJECT_ID,
      project_name: 'sandbox',
      version_number: 2,
      created_at: minutesAgo(31),
      started_at: minutesAgo(30),
      completed_at: minutesAgo(30 - 42 / 60),
      duration_ms: 42_000,
      error: 'pixi install failed',
      logs: 'Running: pixi install -v\nError: failed to fetch tensorflow\nCleaning up\n',
    },
    {
      ...mockJobDetail,
      id: JOB_ID,
      started_at: minutesAgo(180),
      completed_at: minutesAgo(179.5),
      created_at: minutesAgo(181),
    },
  ];
}

const summary = ({ logs: _logs, ...rest }: JobDetail): JobSummary => rest;

export const jobsHandlers = [
  http.get(`${C}/jobs`, ({ request }) => {
    const p = new URL(request.url).searchParams;
    const status = p.get('status')?.split(',');
    const type = p.get('type')?.split(',');
    const project = p.get('project_id');
    const limit = Number(p.get('limit') ?? 0);
    const started = (j: JobSummary) => Date.parse(j.started_at ?? j.created_at);
    let list = mockJobList()
      .filter((j) => !status || status.includes(j.status))
      .filter((j) => !type || type.includes(j.type))
      .filter((j) => !project || j.project_id === project)
      .sort((a, b) =>
        p.get('sort') === 'started_at'
          ? started(a) - started(b)
          : started(b) - started(a),
      )
      .map(summary);
    if (limit > 0) list = list.slice(0, limit);
    return HttpResponse.json(list);
  }),
  http.get(`${C}/jobs/:jobId/logs/stream`, ({ params }) => {
    const job = mockJobList().find((j) => j.id === params.jobId);
    const body = `data: ${(job?.logs ?? '').replace(/\n/g, '\ndata: ')}\n\nevent: done\ndata: ${job?.status ?? 'completed'}\n\n`;
    return new HttpResponse(body, {
      headers: { 'Content-Type': 'text/event-stream' },
    });
  }),
  http.get(`${C}/jobs/:jobId`, ({ params }) => {
    const job = mockJobList().find((j) => j.id === params.jobId);
    if (job) return HttpResponse.json(job);
    return HttpResponse.json({ ...mockJobDetail, id: params.jobId as string });
  }),
  http.post(`${C}/jobs/:jobId/cancel`, ({ params }) => {
    const job =
      mockJobList().find((j) => j.id === params.jobId) ?? mockJobDetail;
    return HttpResponse.json({
      ...summary(job),
      id: params.jobId as string,
      status: 'cancelled',
      completed_at: new Date().toISOString(),
    });
  }),
];
