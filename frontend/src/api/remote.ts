import { apiClient } from '@/api/client';

export interface RemoteServerStatus {
  connected: boolean;
  server_url: string;
  username: string;
}

export interface ConnectServerRequest {
  url: string;
  username: string;
  password: string;
}

export const remoteApi = {
  getServer: () =>
    apiClient.get<RemoteServerStatus>('/remote/server').then((r) => r.data),
  connectServer: (req: ConnectServerRequest) =>
    apiClient.post<RemoteServerStatus>('/remote/connect', req).then((r) => r.data),
  disconnectServer: () =>
    apiClient.delete('/remote/server').then((r) => r.data),

  // Remote workspace endpoints
  listWorkspaces: () =>
    apiClient.get<any[]>('/remote/workspaces').then((r) => r.data),
  getWorkspace: (id: string) =>
    apiClient.get<any>(`/remote/workspaces/${id}`).then((r) => r.data),
  listVersions: (id: string) =>
    apiClient.get<any[]>(`/remote/workspaces/${id}/versions`).then((r) => r.data),
  listTags: (id: string) =>
    apiClient.get<any[]>(`/remote/workspaces/${id}/tags`).then((r) => r.data),
  getPixiToml: (id: string) =>
    apiClient.get<{ content: string }>(`/remote/workspaces/${id}/pixi-toml`).then((r) => r.data),
};
