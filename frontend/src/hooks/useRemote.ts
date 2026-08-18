import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { remoteApi } from '@/api/remote';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';
import type {
  ConnectServerRequest,
  CreateRemoteWorkspaceRequest,
  FederatedIdentityReviewStatusFilter,
} from '@/types';

// Slow down interval polling while the query is errored (e.g. the remote
// server is unreachable) so failed requests don't flash loading UI every
// cycle, but keep retrying so polling self-heals once the server comes back.
export const ERROR_BACKOFF_INTERVAL = 30000;

export const pollWithErrorBackoff =
  (interval: number) =>
  ({ state }: { state: { status: string } }) =>
    state.status === 'error' ? ERROR_BACKOFF_INTERVAL : interval;

export const useRemoteServer = () => {
  return useQuery({
    queryKey: ['remote', 'server'],
    queryFn: remoteApi.getServer,
    // Plain interval, no error backoff: this hits the local backend (a DB
    // read), never the remote server, and its observer in Layout never
    // unmounts — it is the app's only connection-status self-heal, so a
    // transient local error must not stop it.
    refetchInterval: 10000,
  });
};

// Shared view-state derivation so every page gates remote data (and the
// unreachable banner) the same way. Note `status: 'connected'` from the
// backend means a server URL + token are stored — not that the remote is
// actually reachable. Reachability surfaces as errors on the remote data
// queries themselves.
export const useRemoteView = () => {
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const viewMode = useViewModeStore((s) => s.viewMode);
  const { data: serverStatus } = useRemoteServer();
  const isRemoteConnected = isLocalMode && serverStatus?.status === 'connected';
  return {
    isLocalMode,
    viewMode,
    isRemoteConnected,
    isRemoteView: isRemoteConnected && viewMode === 'remote',
  };
};

export const useConnectServer = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (req: ConnectServerRequest) => remoteApi.connectServer(req),
    onSuccess: () => {
      // Reset rather than invalidate: invalidation skips disabled queries, so
      // an errored query left over from a previously unreachable server would
      // keep its stale error (and banner) when reconnecting re-enables it.
      queryClient.resetQueries({ queryKey: ['remote'] });
    },
  });
};

export const useDisconnectServer = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => remoteApi.disconnectServer(),
    onSuccess: () => {
      // Reset for the same reason as useConnectServer: drop any errored state
      // so nothing stale survives into the next connection.
      queryClient.resetQueries({ queryKey: ['remote'] });
    },
  });
};

export const useRemoteWorkspaces = (enabled: boolean) => {
  return useQuery({
    queryKey: ['remote', 'workspaces'],
    queryFn: remoteApi.listWorkspaces,
    enabled,
    refetchInterval: pollWithErrorBackoff(5000),
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
    mutationFn: (req: CreateRemoteWorkspaceRequest) =>
      remoteApi.createWorkspace(req),
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
    refetchInterval: pollWithErrorBackoff(5000), // Poll for job status updates
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

export const useRemoteAuditLogs = (
  enabled: boolean,
  filters?: { user_id?: string; action?: string },
) => {
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
    // Polls (stats are cheap to serve and feed the dashboard tiles), and —
    // more importantly — keeps retrying after an error: this query is part of
    // AdminDashboard's unreachable-banner condition, and without an interval
    // an errored query only refetches on remount, so the banner would stick
    // until the user navigated away even after the server recovered.
    refetchInterval: pollWithErrorBackoff(30000),
  });
};

export const useRemoteFederatedIdentityReviews = (
  enabled: boolean,
  status: FederatedIdentityReviewStatusFilter = 'pending',
) => {
  return useQuery({
    queryKey: ['remote', 'admin', 'federated-identity-reviews', status],
    queryFn: () => remoteApi.listFederatedIdentityReviews(status),
    enabled,
  });
};

export const useApproveRemoteFederatedIdentityReview = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (reviewId: string) =>
      remoteApi.approveFederatedIdentityReview(reviewId),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['remote', 'admin', 'federated-identity-reviews'],
      });
      queryClient.invalidateQueries({
        queryKey: ['remote', 'admin', 'audit-logs'],
      });
    },
  });
};

export const useRejectRemoteFederatedIdentityReview = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (reviewId: string) =>
      remoteApi.rejectFederatedIdentityReview(reviewId),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['remote', 'admin', 'federated-identity-reviews'],
      });
      queryClient.invalidateQueries({
        queryKey: ['remote', 'admin', 'audit-logs'],
      });
    },
  });
};

export const useDiscardRemoteFederatedIdentityReview = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (reviewId: string) =>
      remoteApi.discardFederatedIdentityReview(reviewId),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['remote', 'admin', 'federated-identity-reviews'],
      });
      queryClient.invalidateQueries({
        queryKey: ['remote', 'admin', 'audit-logs'],
      });
    },
  });
};
