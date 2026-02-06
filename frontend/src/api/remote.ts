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
};
