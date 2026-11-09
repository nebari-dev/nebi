// Owner: U8 (jobs).
import type { JobDetail, JobListFilter, JobSummary } from '@/types/contracts';
import { apiClient, CLIENT } from './base';

const toParams = (f: JobListFilter) => ({
  status: f.status?.length ? f.status.join(',') : undefined,
  type: f.type?.length ? f.type.join(',') : undefined,
  project_id: f.project_id,
  sort: f.sort,
  limit: f.limit,
});

export const jobsApi = {
  list: async (filter: JobListFilter = {}): Promise<JobSummary[]> =>
    (await apiClient.get(`${CLIENT}/jobs`, { params: toParams(filter) })).data,
  get: async (id: string): Promise<JobDetail> =>
    (await apiClient.get(`${CLIENT}/jobs/${id}`)).data,
  cancel: async (id: string): Promise<JobSummary> =>
    (await apiClient.post(`${CLIENT}/jobs/${id}/cancel`)).data,
  /** SSE URL for live logs ("data:" chunks, then "event: done"). */
  logStreamUrl: (id: string): string =>
    `${apiClient.defaults.baseURL ?? ''}${CLIENT}/jobs/${id}/logs/stream`,
};
