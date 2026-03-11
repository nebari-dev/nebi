import { apiClient } from './client';
import type {
  RemoteServer,
  ConnectServerRequest,
  RemoteWorkspace,
  RemoteWorkspaceVersion,
  RemoteWorkspaceTag,
  CreateRemoteWorkspaceRequest,
  OCIRegistry,
  Job,
  User,
  AuditLog,
  DashboardStats,
} from '@/types';

export interface AutoConnectConfig {
  remote_url: string;
  auto_connect: boolean;
  already_connected: boolean;
}

export const remoteApi = {
  // Server connection management
  getServer: async (): Promise<RemoteServer> => {
    const { data } = await apiClient.get('/remote/server');
    return data;
  },

  connectServer: async (req: ConnectServerRequest): Promise<RemoteServer> => {
    const { data } = await apiClient.post('/remote/connect', req);
    return data;
  },

  // Connect to remote server using a pre-obtained token (e.g. from device code flow)
  connectWithToken: async (url: string, token: string, username: string): Promise<RemoteServer> => {
    const { data } = await apiClient.post('/remote/connect-with-token', { url, token, username });
    return data;
  },

  // Request a device code from the remote Nebi server (proxied through local backend to avoid CORS)
  requestDeviceCode: async (remoteUrl: string): Promise<{ code: string; expires_in: number }> => {
    const { data } = await apiClient.post('/remote/device-code', { url: remoteUrl });
    return data;
  },

  // Poll the remote Nebi server for device code completion (proxied through local backend)
  pollDeviceCode: async (remoteUrl: string, code: string): Promise<{ status: string; token?: string; username?: string }> => {
    const { data } = await apiClient.get(`/remote/device-code/poll?url=${encodeURIComponent(remoteUrl)}&code=${code}`);
    return data;
  },

  // Get auto-connect configuration (checks NEBI_REMOTE_URL env var)
  getAutoConnectConfig: async (): Promise<AutoConnectConfig> => {
    const { data } = await apiClient.get('/remote/auto-connect-config');
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

  createWorkspace: async (req: CreateRemoteWorkspaceRequest): Promise<RemoteWorkspace> => {
    const { data } = await apiClient.post('/remote/workspaces', req);
    return data;
  },

  deleteWorkspace: async (id: string): Promise<void> => {
    await apiClient.delete(`/remote/workspaces/${id}`);
  },

  // Remote registries proxy
  listRegistries: async (): Promise<OCIRegistry[]> => {
    const { data } = await apiClient.get('/remote/registries');
    return data;
  },

  // Remote jobs proxy
  listJobs: async (): Promise<Job[]> => {
    const { data } = await apiClient.get('/remote/jobs');
    return data;
  },

  // Remote admin proxies
  listUsers: async (): Promise<User[]> => {
    const { data } = await apiClient.get('/remote/admin/users');
    return data;
  },

  listAdminRegistries: async (): Promise<OCIRegistry[]> => {
    const { data } = await apiClient.get('/remote/admin/registries');
    return data;
  },

  listAuditLogs: async (params?: { user_id?: string; action?: string }): Promise<AuditLog[]> => {
    const { data } = await apiClient.get('/remote/admin/audit-logs', { params });
    return data;
  },

  getDashboardStats: async (): Promise<DashboardStats> => {
    const { data } = await apiClient.get('/remote/admin/dashboard/stats');
    return data;
  },
};
