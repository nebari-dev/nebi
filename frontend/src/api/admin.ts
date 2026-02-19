import { apiClient } from './client';
import type { User, CreateUserRequest, AuditLog, Collaborator, ShareWorkspaceRequest, ShareGroupRequest, DashboardStats } from '@/types/models';

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

  // Audit Logs
  getAuditLogs: async (params?: { user_id?: string; action?: string }): Promise<AuditLog[]> => {
    const response = await apiClient.get('/admin/audit-logs', { params });
    return response.data;
  },

  // Workspace Sharing
  getCollaborators: async (workspaceId: string): Promise<Collaborator[]> => {
    const response = await apiClient.get(`/workspaces/${workspaceId}/collaborators`);
    return response.data;
  },

  shareWorkspace: async (workspaceId: string, data: ShareWorkspaceRequest): Promise<void> => {
    await apiClient.post(`/workspaces/${workspaceId}/share`, data);
  },

  unshareWorkspace: async (workspaceId: string, userId: string): Promise<void> => {
    await apiClient.delete(`/workspaces/${workspaceId}/share/${userId}`);
  },

  // Group Sharing
  getGroups: async (): Promise<string[]> => {
    const response = await apiClient.get('/groups');
    return response.data;
  },

  shareWorkspaceWithGroup: async (workspaceId: string, data: ShareGroupRequest): Promise<void> => {
    await apiClient.post(`/workspaces/${workspaceId}/share-group`, data);
  },

  unshareWorkspaceFromGroup: async (workspaceId: string, groupName: string): Promise<void> => {
    await apiClient.delete(`/workspaces/${workspaceId}/share-group/${groupName}`);
  },

  // Dashboard Stats
  getDashboardStats: async (): Promise<DashboardStats> => {
    const response = await apiClient.get('/admin/dashboard/stats');
    return response.data;
  },
};
