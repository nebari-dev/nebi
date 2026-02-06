import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { workspacesApi } from '@/api/workspaces';
import type { RollbackRequest } from '@/types';

export const useVersions = (workspaceId: string) => {
  return useQuery({
    queryKey: ['workspaces', workspaceId, 'versions'],
    queryFn: () => workspacesApi.listVersions(workspaceId),
    enabled: !!workspaceId,
  });
};

export const useVersion = (workspaceId: string, versionNumber: number) => {
  return useQuery({
    queryKey: ['workspaces', workspaceId, 'versions', versionNumber],
    queryFn: () => workspacesApi.getVersion(workspaceId, versionNumber),
    enabled: !!workspaceId && versionNumber > 0,
  });
};

export const useRollback = (workspaceId: string) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: RollbackRequest) => workspacesApi.rollback(workspaceId, data),
    onSuccess: () => {
      // Invalidate relevant queries
      queryClient.invalidateQueries({ queryKey: ['workspaces', workspaceId] });
      queryClient.invalidateQueries({ queryKey: ['workspaces', workspaceId, 'versions'] });
      queryClient.invalidateQueries({ queryKey: ['workspaces', workspaceId, 'packages'] });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};

export const useDownloadLockFile = () => {
  return useMutation({
    mutationFn: ({ workspaceId, versionNumber }: { workspaceId: string; versionNumber: number }) =>
      workspacesApi.downloadLockFile(workspaceId, versionNumber),
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
    mutationFn: ({ workspaceId, versionNumber }: { workspaceId: string; versionNumber: number }) =>
      workspacesApi.downloadManifest(workspaceId, versionNumber),
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

export const useViewLockFile = (workspaceId: string, versionNumber: number, enabled: boolean) => {
  return useQuery({
    queryKey: ['workspaces', workspaceId, 'versions', versionNumber, 'lock-file'],
    queryFn: () => workspacesApi.downloadLockFile(workspaceId, versionNumber),
    enabled: enabled && !!workspaceId && versionNumber > 0,
  });
};

export const useViewManifest = (workspaceId: string, versionNumber: number, enabled: boolean) => {
  return useQuery({
    queryKey: ['workspaces', workspaceId, 'versions', versionNumber, 'manifest'],
    queryFn: () => workspacesApi.downloadManifest(workspaceId, versionNumber),
    enabled: enabled && !!workspaceId && versionNumber > 0,
  });
};
