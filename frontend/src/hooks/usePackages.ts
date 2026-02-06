import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { packagesApi } from '@/api/packages';
import type { InstallPackagesRequest } from '@/types';

export const usePackages = (workspaceId: string) => {
  return useQuery({
    queryKey: ['packages', workspaceId],
    queryFn: () => packagesApi.list(workspaceId),
    enabled: !!workspaceId,
    refetchInterval: 2000,
  });
};

export const useInstallPackages = (workspaceId: string) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: InstallPackagesRequest) => packagesApi.install(workspaceId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['packages', workspaceId] });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};

export const useRemovePackage = (workspaceId: string) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (packageName: string) => packagesApi.remove(workspaceId, packageName),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['packages', workspaceId] });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};
