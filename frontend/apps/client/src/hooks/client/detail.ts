// Owner: U2 (detail).
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { detailApi } from '@/api/client/detail';
import { queryKeys } from '@/lib/queryKeys';
import type { InstallRequest } from '@/types/contracts';

export const useVersions = (projectId: string) =>
  useQuery({
    queryKey: queryKeys.versions(projectId),
    queryFn: () => detailApi.versions(projectId),
    enabled: !!projectId,
  });

export const useVersion = (projectId: string, n: number | undefined) =>
  useQuery({
    queryKey: queryKeys.version(projectId, n ?? 0),
    queryFn: () => detailApi.version(projectId, n as number),
    enabled: !!projectId && !!n,
  });

export const useVersionFile = (
  projectId: string,
  n: number | undefined,
  name: 'pixi.toml' | 'pixi.lock',
  enabled = true,
) =>
  useQuery({
    queryKey: queryKeys.versionFile(projectId, n ?? 0, name),
    queryFn: () => detailApi.file(projectId, n as number, name),
    enabled: enabled && !!projectId && !!n,
  });

export const useResolvedPackages = (projectId: string, n: number | undefined) =>
  useQuery({
    queryKey: queryKeys.resolvedPackages(projectId, n ?? 0),
    queryFn: () => detailApi.resolvedPackages(projectId, n as number),
    enabled: !!projectId && !!n,
  });

/** Install a version (no version = last installed, else latest). */
export const useInstall = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      projectId,
      ...req
    }: InstallRequest & { projectId: string }) =>
      detailApi.install(projectId, req),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.projects }),
  });
};

export const useUninstall = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (projectId: string) => detailApi.uninstall(projectId),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.projects }),
  });
};
