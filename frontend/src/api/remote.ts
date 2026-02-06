import { apiClient } from './client';
import type {
  RemoteServer,
  ConnectServerRequest,
  RemoteWorkspace,
  RemoteWorkspaceVersion,
  RemoteWorkspaceTag,
} from '@/types';

export const remoteApi = {
  // Server connection management
  getServer: async (): Promise<RemoteServer> => {
    const { data } = await apiClient.get('/remote/server');
    return data;
  },

  connectServer: async (req: ConnectServerRequest): Promise<RemoteServer> => {
    const { data } = await apiClient.post('/remote/server', req);
    return data;
  },

  disconnectServer: async (): Promise<void> => {
    await apiClient.delete('/remote/server');
  },

  // Remote workspace proxies
  listWorkspaces: async (): Promise<RemoteWorkspace[]> => {
    const { data } = await apiClient.get('/remote/workspaces');
    return data;
  },

  getWorkspace: async (id: string): Promise<RemoteWorkspace> => {
    const { data } = await apiClient.get(`/remote/workspaces/${id}`);
    return data;
  },

  listVersions: async (id: string): Promise<RemoteWorkspaceVersion[]> => {
    const { data } = await apiClient.get(`/remote/workspaces/${id}/versions`);
    return data;
  },

  listTags: async (id: string): Promise<RemoteWorkspaceTag[]> => {
    const { data } = await apiClient.get(`/remote/workspaces/${id}/tags`);
    return data;
  },

  getPixiToml: async (id: string): Promise<{ content: string }> => {
    const { data } = await apiClient.get(`/remote/workspaces/${id}/pixi-toml`);
    return data;
  },

  getVersionPixiToml: async (id: string, version: number): Promise<string> => {
    const { data } = await apiClient.get(`/remote/workspaces/${id}/versions/${version}/pixi-toml`, {
      responseType: 'text',
    });
    return data;
  },

  getVersionPixiLock: async (id: string, version: number): Promise<string> => {
    const { data } = await apiClient.get(`/remote/workspaces/${id}/versions/${version}/pixi-lock`, {
      responseType: 'text',
    });
    return data;
  },
};
