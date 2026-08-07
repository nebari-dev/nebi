import type { BuildEnvVar, UpsertBuildEnvVarRequest } from '@/types';
import { apiClient } from './client';

export type BuildEnvTarget = 'local' | 'remote';

const buildEnvBasePath = (target: BuildEnvTarget) =>
  target === 'remote' ? '/remote/build-env-vars' : '/build-env-vars';

export const buildEnvApi = {
  list: async (target: BuildEnvTarget): Promise<BuildEnvVar[]> => {
    const { data } = await apiClient.get(buildEnvBasePath(target));
    return data;
  },

  upsert: async (
    target: BuildEnvTarget,
    req: UpsertBuildEnvVarRequest,
  ): Promise<BuildEnvVar> => {
    const { data } = await apiClient.put(buildEnvBasePath(target), req);
    return data;
  },

  delete: async (target: BuildEnvTarget, key: string): Promise<void> => {
    await apiClient.delete(
      `${buildEnvBasePath(target)}/${encodeURIComponent(key)}`,
    );
  },
};
