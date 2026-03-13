import { apiClient } from './client';
import type { ServerConfig } from '@/types';

export const configApi = {
  get: async (): Promise<ServerConfig> => {
    const { data } = await apiClient.get('/version');
    return {
      mode: data.mode,
      features: data.features,
    };
  },
};
