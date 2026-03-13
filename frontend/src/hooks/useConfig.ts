import { useQuery } from '@tanstack/react-query';
import { configApi } from '@/api/config';

export const useServerConfig = () => {
  return useQuery({
    queryKey: ['config'],
    queryFn: () => configApi.get(),
    staleTime: Infinity,
  });
};
