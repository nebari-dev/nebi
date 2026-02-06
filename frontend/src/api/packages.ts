import { apiClient } from './client';
import type { Package, InstallPackagesRequest } from '@/types';

export const packagesApi = {
  list: async (environmentId: string): Promise<Package[]> => {
    const { data } = await apiClient.get(`/workspaces/${environmentId}/packages`);
    return data;
  },

  install: async (environmentId: string, req: InstallPackagesRequest): Promise<void> => {
    await apiClient.post(`/workspaces/${environmentId}/packages`, req);
  },

  remove: async (environmentId: string, packageName: string): Promise<void> => {
    await apiClient.delete(`/workspaces/${environmentId}/packages/${packageName}`);
  },
};
