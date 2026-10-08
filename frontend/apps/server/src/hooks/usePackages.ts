import { useQuery } from '@tanstack/react-query';
import { packagesApi } from '@/api/packages';

export const usePackages = (environmentId: string) => {
  return useQuery({
    queryKey: ['packages', environmentId],
    queryFn: () => packagesApi.list(environmentId),
    enabled: !!environmentId,
    refetchInterval: 2000,
  });
};
