// Owner: U5 (remote).
import type {
  ConnectRequest,
  PullRequest,
  PullResult,
  PushResult,
  RemoteDetails,
  ServerConnection,
  ServerProjectList,
  VersionItem,
} from '@/types/contracts';
import { apiClient, CLIENT } from './base';

export const remoteApi = {
  connection: async (): Promise<ServerConnection> =>
    (await apiClient.get(`${CLIENT}/remote/server`)).data,
  connect: async (req: ConnectRequest): Promise<ServerConnection> =>
    (await apiClient.put(`${CLIENT}/remote/server`, req)).data,
  disconnect: async (): Promise<void> => {
    await apiClient.delete(`${CLIENT}/remote/server`);
  },
  projects: async (search = ''): Promise<ServerProjectList> =>
    (await apiClient.get(`${CLIENT}/remote/projects`, { params: { search } }))
      .data,
  refresh: async (search = ''): Promise<ServerProjectList> =>
    (
      await apiClient.post(`${CLIENT}/remote/projects/refresh`, null, {
        params: { search },
      })
    ).data,
  details: async (serverProjectId: string): Promise<RemoteDetails> =>
    (await apiClient.get(`${CLIENT}/remote/projects/${serverProjectId}`)).data,
  pull: async (
    serverProjectId: string,
    req: PullRequest,
  ): Promise<PullResult> =>
    (
      await apiClient.post(
        `${CLIENT}/remote/projects/${serverProjectId}/pull`,
        req,
      )
    ).data,
  push: async (projectId: string): Promise<PushResult> =>
    (await apiClient.post(`${CLIENT}/projects/${projectId}/push`)).data,
  pullVersion: async (projectId: string, n: number): Promise<VersionItem> =>
    (await apiClient.post(`${CLIENT}/projects/${projectId}/versions/${n}/pull`))
      .data,
};
