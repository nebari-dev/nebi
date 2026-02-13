import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { remoteApi } from '@/api/remote';
import type { ConnectServerRequest, CreateRemoteWorkspaceRequest } from '@/types';

export const useRemoteServer = () => {
  return useQuery({
    queryKey: ['remote', 'server'],
    queryFn: remoteApi.getServer,
    refetchInterval: 10000, // Check connection status every 10s
  });
};

export const useConnectServer = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (req: ConnectServerRequest) => remoteApi.connectServer(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['remote'] });
    },
  });
};

export const useDisconnectServer = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => remoteApi.disconnectServer(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['remote'] });
    },
  });
};

export const useRemoteWorkspaces = (enabled: boolean) => {
  return useQuery({
    queryKey: ['remote', 'workspaces'],
    queryFn: remoteApi.listWorkspaces,
    enabled,
    refetchInterval: 5000,
  });
};

export const useRemoteWorkspace = (id: string) => {
  return useQuery({
    queryKey: ['remote', 'workspaces', id],
    queryFn: () => remoteApi.getWorkspace(id),
    enabled: !!id,
  });
};

export const useRemoteVersions = (wsId: string) => {
  return useQuery({
    queryKey: ['remote', 'workspaces', wsId, 'versions'],
    queryFn: () => remoteApi.listVersions(wsId),
    enabled: !!wsId,
  });
};

export const useRemoteTags = (wsId: string) => {
  return useQuery({
    queryKey: ['remote', 'workspaces', wsId, 'tags'],
    queryFn: () => remoteApi.listTags(wsId),
    enabled: !!wsId,
  });
};

export const useCreateRemoteWorkspace = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (req: CreateRemoteWorkspaceRequest) => remoteApi.createWorkspace(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['remote', 'workspaces'] });
    },
  });
};

export const useDeleteRemoteWorkspace = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => remoteApi.deleteWorkspace(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['remote', 'workspaces'] });
    },
  });
};

export const useRemoteRegistries = (enabled: boolean) => {
  return useQuery({
    queryKey: ['remote', 'registries'],
    queryFn: remoteApi.listRegistries,
    enabled,
  });
};

export const useRemoteJobs = (enabled: boolean) => {
  return useQuery({
    queryKey: ['remote', 'jobs'],
    queryFn: remoteApi.listJobs,
    enabled,
    refetchInterval: 5000, // Poll for job status updates
  });
};

export const useRemoteUsers = (enabled: boolean) => {
  return useQuery({
    queryKey: ['remote', 'admin', 'users'],
    queryFn: remoteApi.listUsers,
    enabled,
  });
};

export const useRemoteAdminRegistries = (enabled: boolean) => {
  return useQuery({
    queryKey: ['remote', 'admin', 'registries'],
    queryFn: remoteApi.listAdminRegistries,
    enabled,
  });
};

export const useRemoteAuditLogs = (enabled: boolean, filters?: { user_id?: string; action?: string }) => {
  return useQuery({
    queryKey: ['remote', 'admin', 'audit-logs', filters],
    queryFn: () => remoteApi.listAuditLogs(filters),
    enabled,
  });
};

export const useRemoteDashboardStats = (enabled: boolean) => {
  return useQuery({
    queryKey: ['remote', 'admin', 'dashboard', 'stats'],
    queryFn: remoteApi.getDashboardStats,
    enabled,
  });
};
