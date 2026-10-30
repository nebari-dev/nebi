import type {
  AuditLog,
  Collaborator,
  DashboardStats,
  Group,
  ShareProjectRequest,
  User,
} from '@/types/models';
import { apiClient } from './client';

export const adminApi = {
  // User Management
  getUsers: async (): Promise<User[]> => {
    const response = await apiClient.get('/admin/users');
    return response.data;
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

  // Project Sharing
  getCollaborators: async (projectId: string): Promise<Collaborator[]> => {
    const response = await apiClient.get(
      `/projects/${projectId}/collaborators`,
    );
    return response.data;
  },

  shareProject: async (
    projectId: string,
    data: ShareProjectRequest,
  ): Promise<void> => {
    await apiClient.post(`/projects/${projectId}/share`, data);
  },

  unshareProject: async (projectId: string, userId: string): Promise<void> => {
    await apiClient.delete(`/projects/${projectId}/share/${userId}`);
  },

  // Dashboard Stats
  getDashboardStats: async (): Promise<DashboardStats> => {
    const response = await apiClient.get('/admin/dashboard/stats');
    return response.data;
  },
};
