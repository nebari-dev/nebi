// Owner: U7 (publish).
import type {
  AddRegistryRequest,
  PublishDefault,
  PublishRequest,
  PublishResult,
  PullResult,
  RegistryItem,
  RegistryPullRequest,
} from '@/types/contracts';
import { apiClient, CLIENT } from './base';

export const publishApi = {
  registries: async (): Promise<RegistryItem[]> =>
    (await apiClient.get(`${CLIENT}/registries`)).data,
  addRegistry: async (req: AddRegistryRequest): Promise<RegistryItem> =>
    (await apiClient.post(`${CLIENT}/registries`, req)).data,
  removeRegistry: async (id: string): Promise<void> => {
    await apiClient.delete(`${CLIENT}/registries/${id}`);
  },
  repositories: async (id: string, search = ''): Promise<string[]> =>
    (
      await apiClient.get(`${CLIENT}/registries/${id}/repositories`, {
        params: { search },
      })
    ).data,
  tags: async (id: string, repository: string): Promise<string[]> =>
    (
      await apiClient.get(`${CLIENT}/registries/${id}/tags`, {
        params: { repository },
      })
    ).data,
  pull: async (id: string, req: RegistryPullRequest): Promise<PullResult> =>
    (await apiClient.post(`${CLIENT}/registries/${id}/pull`, req)).data,
  defaults: async (projectId: string, n: number): Promise<PublishDefault[]> =>
    (
      await apiClient.get(
        `${CLIENT}/projects/${projectId}/versions/${n}/publish-defaults`,
      )
    ).data,
  publish: async (
    projectId: string,
    n: number,
    req: PublishRequest,
  ): Promise<PublishResult> =>
    (
      await apiClient.post(
        `${CLIENT}/projects/${projectId}/versions/${n}/publish`,
        req,
      )
    ).data,
  unpublish: async (
    projectId: string,
    publicationId: string,
  ): Promise<void> => {
    await apiClient.delete(
      `${CLIENT}/projects/${projectId}/publications/${publicationId}`,
    );
  },
};
