import type { AuthConfig, User } from '@/types';
import { apiClient } from './client';

export const authApi = {
  // Public: how this server authenticates requests.
  getConfig: async (): Promise<AuthConfig> => {
    const { data } = await apiClient.get('/auth/config');
    return data;
  },

  // The Nebi user behind the current credentials (created on first request).
  me: async (): Promise<User> => {
    const { data } = await apiClient.get('/auth/me');
    return data;
  },
};
