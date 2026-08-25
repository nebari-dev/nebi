import type {
  AuditLog,
  ConnectServerRequest,
  CreateRegistryRequest,
  CreateRemoteWorkspaceRequest,
  DashboardStats,
  FederatedIdentity,
  FederatedIdentityReview,
  FederatedIdentityReviewStatusFilter,
  Job,
  OCIRegistry,
  RemoteServer,
  RemoteWorkspace,
  RemoteWorkspaceTag,
  RemoteWorkspaceVersion,
  UpdateRegistryRequest,
  User,
} from '@/types';
import { apiClient } from './client';

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
    const { data } = await apiClient.get(
      `/remote/workspaces/${id}/versions/${version}/pixi-toml`,
      {
        responseType: 'text',
      },
    );
    return data;
  },

  getVersionPixiLock: async (id: string, version: number): Promise<string> => {
    const { data } = await apiClient.get(
      `/remote/workspaces/${id}/versions/${version}/pixi-lock`,
      {
        responseType: 'text',
      },
    );
    return data;
  },

  createWorkspace: async (
    req: CreateRemoteWorkspaceRequest,
  ): Promise<RemoteWorkspace> => {
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

  createAdminRegistry: async (
    req: CreateRegistryRequest,
  ): Promise<OCIRegistry> => {
    const { data } = await apiClient.post('/remote/admin/registries', req);
    return data;
  },

  updateAdminRegistry: async (
    id: string,
    req: UpdateRegistryRequest,
  ): Promise<OCIRegistry> => {
    const { data } = await apiClient.put(`/remote/admin/registries/${id}`, req);
    return data;
  },

  deleteAdminRegistry: async (id: string): Promise<void> => {
    await apiClient.delete(`/remote/admin/registries/${id}`);
  },

  listAuditLogs: async (params?: {
    user_id?: string;
    action?: string;
  }): Promise<AuditLog[]> => {
    const { data } = await apiClient.get('/remote/admin/audit-logs', {
      params,
    });
    return data;
  },

  getDashboardStats: async (): Promise<DashboardStats> => {
    const { data } = await apiClient.get('/remote/admin/dashboard/stats');
    return data;
  },

  listFederatedIdentityReviews: async (
    status: FederatedIdentityReviewStatusFilter = 'pending',
  ): Promise<FederatedIdentityReview[]> => {
    const { data } = await apiClient.get(
      '/remote/admin/federated-identity-reviews',
      { params: { status } },
    );
    return data;
  },

  approveFederatedIdentityReview: async (
    reviewId: string,
  ): Promise<FederatedIdentity> => {
    const { data } = await apiClient.post(
      `/remote/admin/federated-identity-reviews/${reviewId}/approve`,
    );
    return data;
  },

  rejectFederatedIdentityReview: async (reviewId: string): Promise<void> => {
    await apiClient.post(
      `/remote/admin/federated-identity-reviews/${reviewId}/reject`,
    );
  },

  discardFederatedIdentityReview: async (reviewId: string): Promise<void> => {
    await apiClient.delete(
      `/remote/admin/federated-identity-reviews/${reviewId}`,
    );
  },
};
