import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { adminApi } from '@/api/admin';
import type { CreateUserRequest, ShareEnvironmentRequest } from '@/types/models';

// Check if current user is admin
export const useIsAdmin = () => {
  return useQuery({
    queryKey: ['user', 'is_admin'],
    queryFn: async () => {
      try {
        await adminApi.getUsers();
        return true;
      } catch {
        return false;
      }
    },
    retry: false,
  });
};

// User Management Hooks
export const useUsers = () => {
  return useQuery({
    queryKey: ['admin', 'users'],
    queryFn: adminApi.getUsers,
  });
};

export const useCreateUser = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateUserRequest) => adminApi.createUser(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
    },
  });
};

export const useToggleAdmin = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) => adminApi.toggleAdmin(userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
    },
  });
};

export const useDeleteUser = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) => adminApi.deleteUser(userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
    },
  });
};

// Audit Logs Hooks
export const useAuditLogs = (filters?: { user_id?: string; action?: string }) => {
  return useQuery({
    queryKey: ['admin', 'audit-logs', filters],
    queryFn: () => adminApi.getAuditLogs(filters),
  });
};

// Collaborators Hooks
export const useCollaborators = (environmentId: string, enabled = true) => {
  return useQuery({
    queryKey: ['collaborators', environmentId],
    queryFn: () => adminApi.getCollaborators(environmentId),
    enabled,
  });
};

export const useShareEnvironment = (environmentId: string) => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: ShareEnvironmentRequest) =>
      adminApi.shareEnvironment(environmentId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['collaborators', environmentId] });
    },
  });
};

export const useUnshareEnvironment = (environmentId: string) => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) =>
      adminApi.unshareEnvironment(environmentId, userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['collaborators', environmentId] });
    },
  });
};

// Dashboard Stats Hooks
export const useDashboardStats = () => {
  return useQuery({
    queryKey: ['admin', 'dashboard', 'stats'],
    queryFn: adminApi.getDashboardStats,
  });
};
