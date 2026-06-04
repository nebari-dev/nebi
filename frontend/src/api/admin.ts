import type {
  AuditLog,
  Collaborator,
  CreateUserRequest,
  DashboardStats,
  Group,
  ShareWorkspaceRequest,
  User,
} from '@/types/models';
import { apiClient } from './client';

export const adminApi = {
  // User Management
  getUsers: async (): Promise<User[]> => {
    const response = await apiClient.get('/admin/users');
    return response.data;
  },

  createUser: async (data: CreateUserRequest): Promise<User> => {
    const response = await apiClient.post('/admin/users', data);
    return response.data;
  },

  toggleAdmin: async (userId: string): Promise<void> => {
    await apiClient.post(`/admin/users/${userId}/toggle-admin`);
  },

  deleteUser: async (userId: string): Promise<void> => {
    await apiClient.delete(`/admin/users/${userId}`);
  },

  getUserGroups: async (userId: string): Promise<Group[]> => {
    const r = await apiClient.get(`/admin/users/${userId}/groups`);
    return r.data;
  },

  // Audit Logs
  getAuditLogs: async (params?: {
    user_id?: string;
    action?: string;
  }): Promise<AuditLog[]> => {
    const response = await apiClient.get('/admin/audit-logs', { params });
    return response.data;
  },

  // Workspace Sharing
  getCollaborators: async (workspaceId: string): Promise<Collaborator[]> => {
    const response = await apiClient.get(
      `/workspaces/${workspaceId}/collaborators`,
    );
    return response.data;
  },

  shareWorkspace: async (
    workspaceId: string,
    data: ShareWorkspaceRequest,
  ): Promise<void> => {
    await apiClient.post(`/workspaces/${workspaceId}/share`, data);
  },

  unshareWorkspace: async (
    workspaceId: string,
    userId: string,
  ): Promise<void> => {
    await apiClient.delete(`/workspaces/${workspaceId}/share/${userId}`);
  },

  // Dashboard Stats
  getDashboardStats: async (): Promise<DashboardStats> => {
    const response = await apiClient.get('/admin/dashboard/stats');
    return response.data;
  },
};
