import type {
  AuditLog,
  ConnectServerRequest,
  CreateRegistryRequest,
  CreateRemoteProjectRequest,
  DashboardStats,
  FederatedIdentity,
  FederatedIdentityReview,
  FederatedIdentityReviewStatusFilter,
  Job,
  OCIRegistry,
  RemoteProject,
  RemoteProjectTag,
  RemoteProjectVersion,
  RemoteServer,
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

  // Remote project proxies
  listProjects: async (): Promise<RemoteProject[]> => {
    const { data } = await apiClient.get('/remote/projects');
    return data;
  },

  getProject: async (id: string): Promise<RemoteProject> => {
    const { data } = await apiClient.get(`/remote/projects/${id}`);
    return data;
  },

  listVersions: async (id: string): Promise<RemoteProjectVersion[]> => {
    const { data } = await apiClient.get(`/remote/projects/${id}/versions`);
    return data;
  },

  listTags: async (id: string): Promise<RemoteProjectTag[]> => {
    const { data } = await apiClient.get(`/remote/projects/${id}/tags`);
    return data;
  },

  getPixiToml: async (id: string): Promise<{ content: string }> => {
    const { data } = await apiClient.get(`/remote/projects/${id}/pixi-toml`);
    return data;
  },

  getVersionPixiToml: async (id: string, version: number): Promise<string> => {
    const { data } = await apiClient.get(
      `/remote/projects/${id}/versions/${version}/pixi-toml`,
      {
        responseType: 'text',
      },
    );
    return data;
  },

  getVersionPixiLock: async (id: string, version: number): Promise<string> => {
    const { data } = await apiClient.get(
      `/remote/projects/${id}/versions/${version}/pixi-lock`,
      {
        responseType: 'text',
      },
    );
    return data;
  },

  createProject: async (
    req: CreateRemoteProjectRequest,
  ): Promise<RemoteProject> => {
    const { data } = await apiClient.post('/remote/projects', req);
    return data;
  },

  deleteProject: async (id: string): Promise<void> => {
    await apiClient.delete(`/remote/projects/${id}`);
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
