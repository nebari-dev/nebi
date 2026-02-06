import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { workspacesApi } from '@/api/workspaces';
import type { RollbackRequest } from '@/types';

export const useVersions = (environmentId: string) => {
  return useQuery({
    queryKey: ['workspaces', environmentId, 'versions'],
    queryFn: () => workspacesApi.listVersions(environmentId),
    enabled: !!environmentId,
  });
};

export const useVersion = (environmentId: string, versionNumber: number) => {
  return useQuery({
    queryKey: ['workspaces', environmentId, 'versions', versionNumber],
    queryFn: () => workspacesApi.getVersion(environmentId, versionNumber),
    enabled: !!environmentId && versionNumber > 0,
  });
};

export const useRollback = (environmentId: string) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: RollbackRequest) => workspacesApi.rollback(environmentId, data),
    onSuccess: () => {
      // Invalidate relevant queries
      queryClient.invalidateQueries({ queryKey: ['workspaces', environmentId] });
      queryClient.invalidateQueries({ queryKey: ['workspaces', environmentId, 'versions'] });
      queryClient.invalidateQueries({ queryKey: ['workspaces', environmentId, 'packages'] });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};

export const useDownloadLockFile = () => {
  return useMutation({
    mutationFn: ({ environmentId, versionNumber }: { environmentId: string; versionNumber: number }) =>
      workspacesApi.downloadLockFile(environmentId, versionNumber),
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
      workspacesApi.downloadManifest(environmentId, versionNumber),
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
    queryKey: ['workspaces', environmentId, 'versions', versionNumber, 'lock-file'],
    queryFn: () => workspacesApi.downloadLockFile(environmentId, versionNumber),
    enabled: enabled && !!environmentId && versionNumber > 0,
  });
};

export const useViewManifest = (environmentId: string, versionNumber: number, enabled: boolean) => {
  return useQuery({
    queryKey: ['workspaces', environmentId, 'versions', versionNumber, 'manifest'],
    queryFn: () => workspacesApi.downloadManifest(environmentId, versionNumber),
    enabled: enabled && !!environmentId && versionNumber > 0,
  });
};
