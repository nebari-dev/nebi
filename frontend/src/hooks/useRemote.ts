import { useEffect, useRef } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { remoteApi } from '@/api/remote';
import type { ConnectServerRequest, CreateRemoteWorkspaceRequest } from '@/types';
import { useViewModeStore } from '@/store/viewModeStore';

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

/**
 * Auto-connect to the remote Nebi server when NEBI_REMOTE_URL is configured.
 *
 * This hook runs once on app load. It checks if NEBI_REMOTE_URL is set (via
 * the /remote/auto-connect-config endpoint) and whether we're already connected.
 * If not connected, it calls /remote/connect-via-proxy which reads the IdToken
 * cookie forwarded by jupyter-server-proxy and exchanges it for a Nebi JWT
 * on the remote server.
 *
 * The result is zero-click auto-connection for JupyterLab users.
 */
export const useAutoConnect = () => {
  const queryClient = useQueryClient();
  const attempted = useRef(false);
  const setViewMode = useViewModeStore((s) => s.setViewMode);

  const { data: config } = useQuery({
    queryKey: ['remote', 'auto-connect-config'],
    queryFn: remoteApi.getAutoConnectConfig,
    staleTime: Infinity, // Only fetch once
    retry: false,
  });

  useEffect(() => {
    if (!config || attempted.current) return;
    if (!config.auto_connect) return;
    if (config.already_connected) {
      // Already connected — just switch to remote view
      setViewMode('remote');
      return;
    }

    attempted.current = true;

    remoteApi.connectViaProxy(config.remote_url).then(() => {
      queryClient.invalidateQueries({ queryKey: ['remote'] });
      setViewMode('remote');
      console.log('[nebi] Auto-connected to remote server:', config.remote_url);
    }).catch((err) => {
      console.warn('[nebi] Auto-connect failed (will retry on next page load):', err.message || err);
    });
  }, [config, queryClient, setViewMode]);
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
