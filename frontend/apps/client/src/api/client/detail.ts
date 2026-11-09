// Owner: U2 (detail). Versions, files, resolved packages, install/uninstall.
import type {
  FileContent,
  InstallRequest,
  JobSummary,
  ResolvedPackages,
  VersionDetail,
  VersionItem,
} from '@/types/contracts';
import { apiClient, CLIENT } from './base';

export const detailApi = {
  versions: async (projectId: string): Promise<VersionItem[]> =>
    (await apiClient.get(`${CLIENT}/projects/${projectId}/versions`)).data,
  version: async (projectId: string, n: number): Promise<VersionDetail> =>
    (await apiClient.get(`${CLIENT}/projects/${projectId}/versions/${n}`)).data,
  file: async (
    projectId: string,
    n: number,
    name: 'pixi.toml' | 'pixi.lock',
  ): Promise<FileContent> =>
    (
      await apiClient.get(
        `${CLIENT}/projects/${projectId}/versions/${n}/files/${name}`,
      )
    ).data,
  resolvedPackages: async (
    projectId: string,
    n: number,
  ): Promise<ResolvedPackages> =>
    (
      await apiClient.get(
        `${CLIENT}/projects/${projectId}/versions/${n}/packages`,
      )
    ).data,
  /** URL for the archive download (use as an <a href download>). */
  archiveUrl: (projectId: string, n: number): string =>
    `${apiClient.defaults.baseURL ?? ''}${CLIENT}/projects/${projectId}/versions/${n}/archive`,
  install: async (
    projectId: string,
    req: InstallRequest = {},
  ): Promise<JobSummary> =>
    (await apiClient.post(`${CLIENT}/projects/${projectId}/install`, req)).data,
  uninstall: async (projectId: string): Promise<JobSummary> =>
    (await apiClient.post(`${CLIENT}/projects/${projectId}/uninstall`)).data,
};
