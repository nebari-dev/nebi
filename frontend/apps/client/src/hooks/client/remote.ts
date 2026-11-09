// Owner: U5 (remote).
//
// Server reachability (AGENTS.md invariants): a 409 "not_connected" means
// no server is stored; a 502 "server_unreachable" means the stored server
// did not answer. Queries that render an unreachable state must keep
// retrying on their own (see pollWithErrorBackoff / retryWhileUnreachable in
// hooks/useRemote.ts), since refetchOnWindowFocus is off app-wide.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { remoteApi } from '@/api/client/remote';
import { queryKeys } from '@/lib/queryKeys';
import type { ConnectRequest, PullRequest } from '@/types/contracts';

/** The connected server ("Connected to", header badge). */
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
      qc.invalidateQueries({ queryKey: ['client', 'remote'] });
      qc.invalidateQueries({ queryKey: queryKeys.projects });
    },
  });
};

export const useServerProjects = (search: string, enabled = true) =>
  useQuery({
    queryKey: queryKeys.serverProjects(search),
    queryFn: () => remoteApi.projects(search),
    enabled,
  });

export const useRefreshServerProjects = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (search: string) => remoteApi.refresh(search),
    onSuccess: (data, search) =>
      qc.setQueryData(queryKeys.serverProjects(search), data),
  });
};

export const useRemoteDetails = (serverProjectId: string) =>
  useQuery({
    queryKey: queryKeys.remoteDetails(serverProjectId),
    queryFn: () => remoteApi.details(serverProjectId),
    enabled: !!serverProjectId,
  });

export const usePullProject = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      serverProjectId,
      ...req
    }: PullRequest & { serverProjectId: string }) =>
      remoteApi.pull(serverProjectId, req),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.projects }),
  });
};

export const usePushProject = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (projectId: string) => remoteApi.push(projectId),
    onSuccess: (_d, projectId) =>
      qc.invalidateQueries({ queryKey: queryKeys.project(projectId) }),
  });
};

export const usePullVersion = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ projectId, n }: { projectId: string; n: number }) =>
      remoteApi.pullVersion(projectId, n),
    onSuccess: (_d, { projectId }) =>
      qc.invalidateQueries({ queryKey: queryKeys.project(projectId) }),
  });
};
