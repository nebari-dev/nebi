import { apiClient } from './client';
import type { Package, InstallPackagesRequest } from '@/types';

export const packagesApi = {
  list: async (environmentId: string): Promise<Package[]> => {
    const { data } = await apiClient.get(`/environments/${environmentId}/packages`);
    return data;
  },

  install: async (environmentId: string, req: InstallPackagesRequest): Promise<void> => {
    await apiClient.post(`/environments/${environmentId}/packages`, req);
  },

  remove: async (environmentId: string, packageName: string): Promise<void> => {
    await apiClient.delete(`/environments/${environmentId}/packages/${packageName}`);
  },
};
