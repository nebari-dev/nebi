import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { remoteApi } from '@/api/remote';
import type { ConnectServerRequest } from '@/api/remote';
import { useModeStore } from '@/store/modeStore';

export const useServerStatus = () => {
  const isLocal = useModeStore((s) => s.mode === 'local');
  return useQuery({
    queryKey: ['remote', 'server'],
    queryFn: remoteApi.getServer,
    enabled: isLocal,
    refetchInterval: 30000, // poll every 30s
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
