import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { type BuildEnvTarget, buildEnvApi } from '@/api/buildEnv';
import type { BuildEnvVar, UpsertBuildEnvVarRequest } from '@/types';

const buildEnvVarsQueryKey = (target: BuildEnvTarget) =>
  target === 'remote'
    ? (['remote', 'build-env-vars'] as const)
    : (['build-env-vars', 'local'] as const);

export const useBuildEnvVars = (target: BuildEnvTarget = 'local') => {
  return useQuery({
    queryKey: buildEnvVarsQueryKey(target),
    queryFn: () => buildEnvApi.list(target),
  });
};

export const useUpsertBuildEnvVar = (target: BuildEnvTarget = 'local') => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (req: UpsertBuildEnvVarRequest) =>
      buildEnvApi.upsert(target, req),
    onSuccess: (saved) => {
      queryClient.setQueryData<BuildEnvVar[]>(
        buildEnvVarsQueryKey(target),
        (current = []) =>
          [...current.filter((envVar) => envVar.key !== saved.key), saved].sort(
            (a, b) => {
              if (a.key < b.key) {
                return -1;
              }
              if (a.key > b.key) {
                return 1;
              }
              return 0;
            },
          ),
      );
    },
  });
};

export const useDeleteBuildEnvVar = (target: BuildEnvTarget = 'local') => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (key: string) => buildEnvApi.delete(target, key),
    onSuccess: (_result, key) => {
      queryClient.setQueryData<BuildEnvVar[]>(
        buildEnvVarsQueryKey(target),
        (current) => current?.filter((envVar) => envVar.key !== key),
      );
    },
  });
};
