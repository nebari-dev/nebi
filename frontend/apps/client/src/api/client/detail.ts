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
  /**
   * Fetches the version archive as a blob (through apiClient, so it works
   * wherever the API needs headers) with the server's suggested file name.
   */
  downloadArchive: async (
    projectId: string,
    n: number,
  ): Promise<{ blob: Blob; fileName: string }> => {
    const res = await apiClient.get(
      `${CLIENT}/projects/${projectId}/versions/${n}/archive`,
      { responseType: 'blob' },
    );
    const disposition = String(res.headers?.['content-disposition'] ?? '');
    return {
      blob: res.data as Blob,
      fileName: archiveFileName(disposition) ?? `version-${n}.tar.gz`,
    };
  },
};

/** File name from a Content-Disposition header (quoted or bare). */
export function archiveFileName(disposition: string): string | null {
  const match = /filename\*?=(?:UTF-8'')?"?([^";]+)"?/i.exec(disposition);
  if (!match?.[1]) return null;
  try {
    return decodeURIComponent(match[1].trim());
  } catch {
    return match[1].trim();
  }
}
