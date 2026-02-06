import { apiClient } from './client';
import type { Package, InstallPackagesRequest } from '@/types';

export const packagesApi = {
  list: async (workspaceId: string): Promise<Package[]> => {
    const { data } = await apiClient.get(`/workspaces/${workspaceId}/packages`);
    return data;
  },

  install: async (workspaceId: string, req: InstallPackagesRequest): Promise<void> => {
    await apiClient.post(`/workspaces/${workspaceId}/packages`, req);
  },

  remove: async (workspaceId: string, packageName: string): Promise<void> => {
    await apiClient.delete(`/workspaces/${workspaceId}/packages/${packageName}`);
  },
};
