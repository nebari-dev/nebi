import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { adminApi } from '@/api/admin';
import type { ShareProjectRequest } from '@/types/models';

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

export const useUserGroups = (userId: string | undefined) =>
  useQuery({
    queryKey: ['admin', 'users', userId, 'groups'],
    queryFn: () => {
      if (!userId) throw new Error('User ID is required');
      return adminApi.getUserGroups(userId);
    },
    enabled: !!userId,
  });

// Audit Logs Hooks
export const useAuditLogs = (filters?: {
  user_id?: string;
  action?: string;
}) => {
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

export const useShareProject = (projectId: string) => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: ShareProjectRequest) =>
      adminApi.shareProject(projectId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['collaborators', projectId],
      });
    },
  });
};

export const useUnshareProject = (projectId: string) => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) => adminApi.unshareProject(projectId, userId),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['collaborators', projectId],
      });
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
