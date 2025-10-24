import { apiClient } from './client';
import type { Environment, CreateEnvironmentRequest } from '@/types';

export const environmentsApi = {
  list: async (): Promise<Environment[]> => {
    const { data } = await apiClient.get('/environments');
    return data;
  },

  get: async (id: string): Promise<Environment> => {
    const { data } = await apiClient.get(`/environments/${id}`);
    return data;
  },

  create: async (req: CreateEnvironmentRequest): Promise<Environment> => {
    const { data } = await apiClient.post('/environments', req);
    return data;
  },

  delete: async (id: string): Promise<void> => {
    await apiClient.delete(`/environments/${id}`);
  },

  getPixiToml: async (id: string): Promise<{ content: string }> => {
    const { data } = await apiClient.get(`/environments/${id}/pixi-toml`);
    return data;
  },
};
