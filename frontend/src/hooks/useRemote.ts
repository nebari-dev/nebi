import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { remoteApi } from '@/api/remote';
import type { ConnectServerRequest } from '@/types';

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
