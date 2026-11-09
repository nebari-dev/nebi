// Owner: U8 (jobs).
import { HttpResponse, http } from 'msw';
import { mockJobDetail, mockJobSummary } from '@/test/fixtures/contracts';

const C = '/api/v1/client';

export const jobsHandlers = [
  http.get(`${C}/jobs`, () => HttpResponse.json([mockJobSummary])),
  http.get(`${C}/jobs/:jobId`, ({ params }) =>
    HttpResponse.json({ ...mockJobDetail, id: params.jobId as string }),
  ),
  http.post(`${C}/jobs/:jobId/cancel`, ({ params }) =>
    HttpResponse.json({
      ...mockJobSummary,
      id: params.jobId as string,
      status: 'cancelled',
    }),
  ),
];
