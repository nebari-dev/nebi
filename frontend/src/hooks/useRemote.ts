import { useEffect, useRef, useCallback } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { remoteApi } from '@/api/remote';
import type { ConnectServerRequest, CreateRemoteWorkspaceRequest } from '@/types';
import { useViewModeStore } from '@/store/viewModeStore';
import { useDeviceCodeStore } from '@/store/deviceCodeStore';

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
 * If not connected, it initiates a device code flow: requests a code from the
 * remote server, exposes an approval URL for the user to click, and polls for
 * completion. Keycloak SSO typically auto-approves with zero interaction.
 *
 * Returns state for the UI to render a "Connect to team server" link.
 */
export const useAutoConnect = () => {
  const queryClient = useQueryClient();
  const attempted = useRef(false);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const setViewMode = useViewModeStore((s) => s.setViewMode);
  const { connecting, approvalUrl, error, setConnecting, setApprovalUrl, setError } = useDeviceCodeStore();

  const { data: config } = useQuery({
    queryKey: ['remote', 'auto-connect-config'],
    queryFn: remoteApi.getAutoConnectConfig,
    staleTime: Infinity, // Only fetch once
    retry: false,
  });

  const startDeviceCodeFlow = useCallback(async (remoteUrl: string) => {
    // Prevent duplicate flows (e.g. hook called from multiple components)
    if (useDeviceCodeStore.getState().connecting) return;
    try {
      setConnecting(true);
      setError(null);

      // Request a device code via the local backend proxy (avoids CORS)
      const { code } = await remoteApi.requestDeviceCode(remoteUrl);

      // Build approval URL and expose it for the UI
      const url = `${remoteUrl}/api/v1/auth/cli-login?code=${code}`;
      setApprovalUrl(url);

      // Open in a new tab — Keycloak SSO may auto-approve
      window.open(url, '_blank');

      // Poll for completion
      pollRef.current = setInterval(async () => {
        try {
          const result = await remoteApi.pollDeviceCode(remoteUrl, code);
          if (result.status === 'complete' && result.token && result.username) {
            clearInterval(pollRef.current!);
            pollRef.current = null;

            // Store credentials locally
            await remoteApi.connectWithToken(remoteUrl, result.token, result.username);
            queryClient.invalidateQueries({ queryKey: ['remote'] });
            setViewMode('remote');
            setApprovalUrl(null);
            setConnecting(false);
            console.log('[nebi] Auto-connected to remote server:', remoteUrl);
          }
        } catch {
          // Poll failed — keep trying until timeout
        }
      }, 2000);

      // Stop polling after 5 minutes
      setTimeout(() => {
        if (pollRef.current) {
          clearInterval(pollRef.current!);
          pollRef.current = null;
          setConnecting(false);
          setError('Connection timed out. Please try again.');
        }
      }, 300000);
    } catch (err) {
      setConnecting(false);
      setError(err instanceof Error ? err.message : 'Failed to start connection');
      console.warn('[nebi] Device code flow failed:', err);
    }
  }, [queryClient, setViewMode, setConnecting, setApprovalUrl, setError]);

  useEffect(() => {
    if (!config || attempted.current) return;
    if (!config.auto_connect) return;
    if (config.already_connected) {
      // Already connected — just switch to remote view
      setViewMode('remote');
      return;
    }

    attempted.current = true;
    startDeviceCodeFlow(config.remote_url);

    return () => {
      if (pollRef.current) {
        clearInterval(pollRef.current);
      }
    };
  }, [config, setViewMode, startDeviceCodeFlow]);

  return { approvalUrl, connecting, error, startDeviceCodeFlow, config };
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
