// Owner: U3 (newversion).
import type {
  CreateVersionRequest,
  JobSummary,
  VersionDiff,
} from '@/types/contracts';
import { apiClient, CLIENT } from './base';

export const newVersionApi = {
  create: async (
    projectId: string,
    req: CreateVersionRequest,
  ): Promise<JobSummary> =>
    (await apiClient.post(`${CLIENT}/projects/${projectId}/versions`, req))
      .data,
  preview: async (
    projectId: string,
    baseVersion: number,
    pixiToml: string,
  ): Promise<VersionDiff> =>
    (
      await apiClient.post(`${CLIENT}/projects/${projectId}/versions/preview`, {
        base_version: baseVersion,
        pixi_toml: pixiToml,
      })
    ).data,
};
