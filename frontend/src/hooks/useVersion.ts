import { useQuery } from '@tanstack/react-query';
import { apiClient } from '@/api/client';

interface VersionInfo {
  version: string;
  commit: string;
  mode: string;
}

export const useVersion = () => {
  return useQuery({
    queryKey: ['version'],
    queryFn: async () => {
      const { data } = await apiClient.get<VersionInfo>('/version');
      return data;
    },
    staleTime: Infinity,
  });
};
