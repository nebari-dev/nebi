import type { Job } from '@/types';
import { apiClient } from './client';

export const jobsApi = {
  list: async (): Promise<Job[]> => {
    const { data } = await apiClient.get('/jobs');
    return data;
  },

  get: async (id: string): Promise<Job> => {
    const { data } = await apiClient.get(`/jobs/${id}`);
    return data;
  },
};
