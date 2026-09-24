import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { projectsApi } from '@/api/projects';
import type { RollbackRequest } from '@/types';

export const useVersions = (environmentId: string) => {
  return useQuery({
    queryKey: ['projects', environmentId, 'versions'],
    queryFn: () => projectsApi.listVersions(environmentId),
    enabled: !!environmentId,
  });
};

export const useVersion = (environmentId: string, versionNumber: number) => {
  return useQuery({
    queryKey: ['projects', environmentId, 'versions', versionNumber],
    queryFn: () => projectsApi.getVersion(environmentId, versionNumber),
    enabled: !!environmentId && versionNumber > 0,
  });
};

export const useRollback = (environmentId: string) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: RollbackRequest) =>
      projectsApi.rollback(environmentId, data),
    onSuccess: () => {
      // Invalidate relevant queries
      queryClient.invalidateQueries({
        queryKey: ['projects', environmentId],
      });
      queryClient.invalidateQueries({
        queryKey: ['projects', environmentId, 'versions'],
      });
      queryClient.invalidateQueries({
        queryKey: ['projects', environmentId, 'packages'],
      });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};

export const useDownloadLockFile = () => {
  return useMutation({
    mutationFn: ({
      environmentId,
      versionNumber,
    }: {
      environmentId: string;
      versionNumber: number;
    }) => projectsApi.downloadLockFile(environmentId, versionNumber),
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
    mutationFn: ({
      environmentId,
      versionNumber,
    }: {
      environmentId: string;
      versionNumber: number;
    }) => projectsApi.downloadManifest(environmentId, versionNumber),
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

export const useViewLockFile = (
  environmentId: string,
  versionNumber: number,
  enabled: boolean,
) => {
  return useQuery({
    queryKey: [
      'projects',
      environmentId,
      'versions',
      versionNumber,
      'lock-file',
    ],
    queryFn: () => projectsApi.downloadLockFile(environmentId, versionNumber),
    enabled: enabled && !!environmentId && versionNumber > 0,
  });
};

export const useViewManifest = (
  environmentId: string,
  versionNumber: number,
  enabled: boolean,
) => {
  return useQuery({
    queryKey: [
      'projects',
      environmentId,
      'versions',
      versionNumber,
      'manifest',
    ],
    queryFn: () => projectsApi.downloadManifest(environmentId, versionNumber),
    enabled: enabled && !!environmentId && versionNumber > 0,
  });
};
