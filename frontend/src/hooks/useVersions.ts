import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { environmentsApi } from '@/api/environments';
import type { RollbackRequest } from '@/types';

export const useVersions = (environmentId: string) => {
  return useQuery({
    queryKey: ['environments', environmentId, 'versions'],
    queryFn: () => environmentsApi.listVersions(environmentId),
    enabled: !!environmentId,
  });
};

export const useVersion = (environmentId: string, versionNumber: number) => {
  return useQuery({
    queryKey: ['environments', environmentId, 'versions', versionNumber],
    queryFn: () => environmentsApi.getVersion(environmentId, versionNumber),
    enabled: !!environmentId && versionNumber > 0,
  });
};

export const useRollback = (environmentId: string) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: RollbackRequest) => environmentsApi.rollback(environmentId, data),
    onSuccess: () => {
      // Invalidate relevant queries
      queryClient.invalidateQueries({ queryKey: ['environments', environmentId] });
      queryClient.invalidateQueries({ queryKey: ['environments', environmentId, 'versions'] });
      queryClient.invalidateQueries({ queryKey: ['environments', environmentId, 'packages'] });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};

export const useDownloadLockFile = () => {
  return useMutation({
    mutationFn: ({ environmentId, versionNumber }: { environmentId: string; versionNumber: number }) =>
      environmentsApi.downloadLockFile(environmentId, versionNumber),
    onSuccess: (data, variables) => {
      // Create a blob and trigger download
      const blob = new Blob([data], { type: 'text/plain' });
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `pixi-lock-v${variables.versionNumber}.lock`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      window.URL.revokeObjectURL(url);
    },
  });
};

export const useDownloadManifest = () => {
  return useMutation({
    mutationFn: ({ environmentId, versionNumber }: { environmentId: string; versionNumber: number }) =>
      environmentsApi.downloadManifest(environmentId, versionNumber),
    onSuccess: (data, variables) => {
      // Create a blob and trigger download
      const blob = new Blob([data], { type: 'text/plain' });
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `pixi-toml-v${variables.versionNumber}.toml`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      window.URL.revokeObjectURL(url);
    },
  });
};

export const useViewLockFile = (environmentId: string, versionNumber: number, enabled: boolean) => {
  return useQuery({
    queryKey: ['environments', environmentId, 'versions', versionNumber, 'lock-file'],
    queryFn: () => environmentsApi.downloadLockFile(environmentId, versionNumber),
    enabled: enabled && !!environmentId && versionNumber > 0,
  });
};

export const useViewManifest = (environmentId: string, versionNumber: number, enabled: boolean) => {
  return useQuery({
    queryKey: ['environments', environmentId, 'versions', versionNumber, 'manifest'],
    queryFn: () => environmentsApi.downloadManifest(environmentId, versionNumber),
    enabled: enabled && !!environmentId && versionNumber > 0,
  });
};
