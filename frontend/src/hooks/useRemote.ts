import {
  type NotifyOnChangeProps,
  type UseQueryResult,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { remoteApi } from '@/api/remote';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';
import type {
  ConnectServerRequest,
  CreateRegistryRequest,
  CreateRemoteWorkspaceRequest,
  FederatedIdentityReviewStatusFilter,
  UpdateRegistryRequest,
} from '@/types';

// Slow down interval polling while the query is errored (e.g. the remote
// server is unreachable) so failed requests don't flash loading UI every
// cycle, but keep retrying so polling self-heals once the server comes back.
export const ERROR_BACKOFF_INTERVAL = 30000;

export const pollWithErrorBackoff =
  (interval: number) =>
  ({ state }: { state: { status: string } }) =>
    state.status === 'error' ? ERROR_BACKOFF_INTERVAL : interval;

// Self-heal for banner-feeding queries that shouldn't poll while healthy: an
// errored query with no refetchInterval only refetches on remount (this app
// sets refetchOnWindowFocus: false globally, so focus is not a recovery path),
// so an unreachable banner gated on its isUnreachable flag would stick even
// after the server recovered. Retry on the error-backoff cadence while
// errored, stay quiet otherwise. (Steady-state freshness polling for these
// queries is a separate product decision — see issue #504.)
export const retryWhileUnreachable = ({
  state,
}: {
  state: { status: string };
}) => (state.status === 'error' ? ERROR_BACKOFF_INTERVAL : false);

export const useRemoteServer = () => {
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  return useQuery({
    queryKey: ['remote', 'server'],
    queryFn: remoteApi.getServer,
    // Plain interval, no error backoff: this hits the local backend (a DB
    // read), never the remote server, and its observer in Layout (via
    // useRemoteView) never unmounts — it is the app's only connection-status
    // self-heal, so a transient local error must not stop it.
    refetchInterval: 10000,
    // The /remote/* endpoints are only registered by the backend in local
    // mode; polling them in remote mode just produces a 404 every cycle.
    enabled: isLocalMode,
  });
};

// Named view-state flags for the remote data queries, so pages consume intent
// instead of re-deriving it from TanStack internals:
// - isFirstLoad: true until the query first resolves or errors — gate the
//   full-page spinner on this. A refetch after an error resets the query to
//   pending, so gating on isLoading alone would flash the spinner on every
//   retry (issue #217).
// - isUnreachable: the query is errored. The backend wraps every remote
//   failure as a 502, so this is how remote reachability surfaces — pages
//   render RemoteUnreachableBanner when this is true in the remote view.
//   Held true while a retry is in flight: that same pending reset clears
//   isError, which would flash the banner off (and the empty state on) for
//   the duration of every failed retry. errorUpdateCount survives the reset,
//   so pending + a past error means "still retrying an unreachable server";
//   a successful retry (or the resetQueries in connect/disconnect) clears it.
// Coupling: any query whose isUnreachable flag a page renders must keep
// retrying on its own — give it pollWithErrorBackoff (if it polls anyway) or
// retryWhileUnreachable (if it shouldn't poll while healthy). Without one,
// the banner sticks after the server recovers until the user navigates away.
const withRemoteFlags = <T>(query: UseQueryResult<T>) => ({
  ...query,
  isFirstLoad: query.isLoading && query.errorUpdateCount === 0,
  isUnreachable:
    query.isError || (query.isPending && query.errorUpdateCount > 0),
});

// Every wrapped query must pin notifyOnChangeProps to exactly the fields the
// flags above (and the pages) read. With it unset, useQuery hands back a proxy
// that marks a field tracked on property access, and the `...query` spread in
// withRemoteFlags accesses all of them — so a poll tick returning unchanged
// data would still re-render every consumer via isFetching/dataUpdatedAt.
// Grow this list if a page ever starts consuming another field (isFetching,
// refetch, error, …); a field left out simply stops triggering re-renders.
const remoteFlagNotifyProps: NotifyOnChangeProps = [
  'data',
  'isLoading',
  'isPending',
  'isError',
  'errorUpdateCount',
];

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
  return withRemoteFlags(
    useQuery({
      queryKey: ['remote', 'workspaces'],
      queryFn: remoteApi.listWorkspaces,
      enabled,
      notifyOnChangeProps: remoteFlagNotifyProps,
      refetchInterval: pollWithErrorBackoff(5000),
    }),
  );
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
  return withRemoteFlags(
    useQuery({
      queryKey: ['remote', 'registries'],
      queryFn: remoteApi.listRegistries,
      enabled,
      notifyOnChangeProps: remoteFlagNotifyProps,
      refetchInterval: retryWhileUnreachable,
    }),
  );
};

export const useRemoteJobs = (enabled: boolean) => {
  return withRemoteFlags(
    useQuery({
      queryKey: ['remote', 'jobs'],
      queryFn: remoteApi.listJobs,
      enabled,
      notifyOnChangeProps: remoteFlagNotifyProps,
      refetchInterval: pollWithErrorBackoff(5000), // Poll for job status updates
    }),
  );
};

export const useRemoteUsers = (enabled: boolean) => {
  return withRemoteFlags(
    useQuery({
      queryKey: ['remote', 'admin', 'users'],
      queryFn: remoteApi.listUsers,
      enabled,
      notifyOnChangeProps: remoteFlagNotifyProps,
      refetchInterval: retryWhileUnreachable,
    }),
  );
};

export const useRemoteAdminRegistries = (enabled: boolean) => {
  return withRemoteFlags(
    useQuery({
      queryKey: ['remote', 'admin', 'registries'],
      queryFn: remoteApi.listAdminRegistries,
      enabled,
      notifyOnChangeProps: remoteFlagNotifyProps,
      refetchInterval: retryWhileUnreachable,
    }),
  );
};

export const useCreateRemoteRegistry = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (req: CreateRegistryRequest) =>
      remoteApi.createAdminRegistry(req),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['remote', 'admin', 'registries'],
      });
    },
  });
};

export const useUpdateRemoteRegistry = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateRegistryRequest }) =>
      remoteApi.updateAdminRegistry(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['remote', 'admin', 'registries'],
      });
    },
  });
};

export const useDeleteRemoteRegistry = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => remoteApi.deleteAdminRegistry(id),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['remote', 'admin', 'registries'],
      });
    },
  });
};

export const useRemoteAuditLogs = (
  enabled: boolean,
  filters?: { user_id?: string; action?: string },
) => {
  return withRemoteFlags(
    useQuery({
      queryKey: ['remote', 'admin', 'audit-logs', filters],
      queryFn: () => remoteApi.listAuditLogs(filters),
      enabled,
      notifyOnChangeProps: remoteFlagNotifyProps,
      refetchInterval: retryWhileUnreachable,
    }),
  );
};

export const useRemoteDashboardStats = (enabled: boolean) => {
  return withRemoteFlags(
    useQuery({
      queryKey: ['remote', 'admin', 'dashboard', 'stats'],
      queryFn: remoteApi.getDashboardStats,
      enabled,
      notifyOnChangeProps: remoteFlagNotifyProps,
      // Polls (stats are cheap to serve and feed the dashboard tiles), and —
      // more importantly — keeps retrying after an error: this query is part
      // of AdminDashboard's unreachable-banner condition, and without an
      // interval an errored query only refetches on remount, so the banner
      // would stick until the user navigated away even after the server
      // recovered.
      refetchInterval: pollWithErrorBackoff(30000),
    }),
  );
};

export const useRemoteFederatedIdentityReviews = (
  enabled: boolean,
  status: FederatedIdentityReviewStatusFilter = 'pending',
) => {
  return withRemoteFlags(
    useQuery({
      queryKey: ['remote', 'admin', 'federated-identity-reviews', status],
      queryFn: () => remoteApi.listFederatedIdentityReviews(status),
      enabled,
      notifyOnChangeProps: remoteFlagNotifyProps,
      refetchInterval: retryWhileUnreachable,
    }),
  );
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
