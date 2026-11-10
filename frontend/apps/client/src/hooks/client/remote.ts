// Owner: U5 (remote).
//
// Server reachability (AGENTS.md invariants): a 409 "not_connected" means
// no server is stored; a 502 "server_unreachable" means the stored server
// did not answer. Queries that render an unreachable state must keep
// retrying on their own (see pollWithErrorBackoff / retryWhileUnreachable in
// hooks/useRemote.ts), since refetchOnWindowFocus is off app-wide.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { clientApiErrorCode, httpStatus } from '@/api/client/base';
import { remoteApi } from '@/api/client/remote';
import { retryWhileUnreachable } from '@/hooks/useRemote';
import { queryKeys } from '@/lib/queryKeys';
import type { PushVariables } from '@/types/client/remote';
import type {
  ConnectRequest,
  PullRequest,
  ServerConnection,
  ServerProjectList,
} from '@/types/contracts';

/** What a remote query error means for the page. */
export type RemoteErrorKind =
  | 'not_connected'
  | 'unreachable'
  | 'auth'
  | 'forbidden'
  | 'other';

export function remoteErrorKind(err: unknown): RemoteErrorKind {
  const code = clientApiErrorCode(err);
  if (code === 'not_connected') return 'not_connected';
  if (code === 'server_auth') return 'auth';
  if (code === 'server_forbidden') return 'forbidden';
  if (
    code === 'server_unreachable' ||
    (httpStatus(err) === undefined && err != null)
  )
    return 'unreachable';
  return 'other';
}

const DISCONNECTED: ServerConnection = {
  connected: false,
  url: '',
  name: '',
  username: '',
  auth: 'none',
  last_synced_at: null,
};

/** The connected server ("Connected to", header badge). A local DB read. */
export const useServerConnection = () =>
  useQuery({
    queryKey: queryKeys.serverConnection,
    queryFn: remoteApi.connection,
    retry: false,
  });

export const useConnectServer = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: ConnectRequest) => remoteApi.connect(req),
    onSuccess: (conn) => {
      qc.setQueryData(queryKeys.serverConnection, conn);
      qc.invalidateQueries({ queryKey: ['client', 'remote'] });
      qc.invalidateQueries({ queryKey: queryKeys.projects });
    },
  });
};

export const useDisconnectServer = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: remoteApi.disconnect,
    onSuccess: () => {
      qc.setQueryData(queryKeys.serverConnection, DISCONNECTED);
      qc.removeQueries({ queryKey: ['client', 'remote', 'projects'] });
      qc.invalidateQueries({ queryKey: ['client', 'remote'] });
      qc.invalidateQueries({ queryKey: queryKeys.projects });
    },
  });
};

/**
 * Server projects. The Server page fetches the whole list once ("" search)
 * and filters it as you type, like the prototype. While the server is
 * unreachable the query keeps retrying on its own (retryWhileUnreachable).
 */
export const useServerProjects = (search: string, enabled = true) =>
  useQuery({
    queryKey: queryKeys.serverProjects(search),
    queryFn: () => remoteApi.projects(search),
    enabled,
    retry: false,
    refetchInterval: retryWhileUnreachable,
  });

export const useRefreshServerProjects = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (search: string) => remoteApi.refresh(search),
    onSuccess: (data: ServerProjectList, search) => {
      qc.setQueryData(queryKeys.serverProjects(search), data);
      qc.setQueryData<ServerConnection>(queryKeys.serverConnection, (prev) =>
        prev ? { ...prev, last_synced_at: data.last_synced_at } : prev,
      );
      // Sync markers and "Update available" may have changed too.
      qc.invalidateQueries({ queryKey: queryKeys.projects });
    },
  });
};

export const useRemoteDetails = (serverProjectId: string) =>
  useQuery({
    queryKey: queryKeys.remoteDetails(serverProjectId),
    queryFn: () => remoteApi.details(serverProjectId),
    enabled: !!serverProjectId,
    retry: false,
    refetchInterval: retryWhileUnreachable,
  });

export const usePullProject = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      serverProjectId,
      ...req
    }: PullRequest & { serverProjectId: string }) =>
      remoteApi.pull(serverProjectId, req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.projects });
      qc.invalidateQueries({ queryKey: ['client', 'remote', 'projects'] });
    },
  });
};

/** Push to server: usePushProject().mutate(projectId) or ({projectId, upTo}). */
export const usePushProject = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: PushVariables) =>
      typeof vars === 'string'
        ? remoteApi.push(vars)
        : remoteApi.push(vars.projectId, vars.upTo),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.projects });
      qc.invalidateQueries({ queryKey: ['client', 'remote', 'projects'] });
    },
  });
};

export const usePullVersion = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ projectId, n }: { projectId: string; n: number }) =>
      remoteApi.pullVersion(projectId, n),
    onSuccess: (_d, { projectId }) => {
      qc.invalidateQueries({ queryKey: queryKeys.project(projectId) });
      qc.invalidateQueries({ queryKey: queryKeys.projects });
    },
  });
};
