import { apiClient } from './client';
import type { User, CreateUserRequest, AuditLog, Collaborator, ShareEnvironmentRequest } from '@/types/models';

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

  // Environment Sharing
  getCollaborators: async (environmentId: string): Promise<Collaborator[]> => {
    const response = await apiClient.get(`/environments/${environmentId}/collaborators`);
    return response.data;
  },

  shareEnvironment: async (environmentId: string, data: ShareEnvironmentRequest): Promise<void> => {
    await apiClient.post(`/environments/${environmentId}/share`, data);
  },

  unshareEnvironment: async (environmentId: string, userId: string): Promise<void> => {
    await apiClient.delete(`/environments/${environmentId}/share/${userId}`);
  },
};
