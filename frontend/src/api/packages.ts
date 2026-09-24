import type { InstallPackagesRequest, Package } from '@/types';
import { apiClient } from './client';

export const packagesApi = {
  list: async (environmentId: string): Promise<Package[]> => {
    const { data } = await apiClient.get(`/projects/${environmentId}/packages`);
    return data;
  },

  install: async (
    environmentId: string,
    req: InstallPackagesRequest,
  ): Promise<void> => {
    await apiClient.post(`/projects/${environmentId}/packages`, req);
  },

  remove: async (environmentId: string, packageName: string): Promise<void> => {
    await apiClient.delete(
      `/projects/${environmentId}/packages/${packageName}`,
    );
  },
};
