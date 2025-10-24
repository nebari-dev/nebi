import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { packagesApi } from '@/api/packages';
import type { InstallPackagesRequest } from '@/types';

export const usePackages = (environmentId: string) => {
  return useQuery({
    queryKey: ['packages', environmentId],
    queryFn: () => packagesApi.list(environmentId),
    enabled: !!environmentId,
    refetchInterval: 2000,
  });
};

export const useInstallPackages = (environmentId: string) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: InstallPackagesRequest) => packagesApi.install(environmentId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['packages', environmentId] });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};

export const useRemovePackage = (environmentId: string) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (packageName: string) => packagesApi.remove(environmentId, packageName),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['packages', environmentId] });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};
