// Owner: U3 (newversion).
import type {
  PreviewChangesRequest,
  VersionDraft,
} from '@/types/client/newversion';
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
  /** Summary of changes of a draft (pixi.toml or form) against its base. */
  preview: async (
    projectId: string,
    req: PreviewChangesRequest,
  ): Promise<VersionDiff> =>
    (
      await apiClient.post(
        `${CLIENT}/projects/${projectId}/versions/preview`,
        req,
      )
    ).data,
  /** Starting point of a new version from `from` ("latest" = newest). */
  draft: async (
    projectId: string,
    from: number | 'latest',
  ): Promise<VersionDraft> =>
    (
      await apiClient.get(
        `${CLIENT}/projects/${projectId}/versions/${from}/draft`,
      )
    ).data,
};
