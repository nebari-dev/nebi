import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { workspacesApi } from '@/api/workspaces';
import type { CreateWorkspaceRequest } from '@/types';

export const useWorkspaces = () => {
  return useQuery({
    queryKey: ['workspaces'],
    queryFn: workspacesApi.list,
    refetchInterval: 2000, // Poll every 2 seconds for status updates
  });
};

export const useWorkspace = (id: string) => {
  return useQuery({
    queryKey: ['workspaces', id],
    queryFn: () => workspacesApi.get(id),
    enabled: !!id,
    refetchInterval: 2000,
  });
};

export const useCreateWorkspace = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateWorkspaceRequest) => workspacesApi.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
    },
  });
};

export const useDeleteWorkspace = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => workspacesApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
    },
  });
};

export const useWorkspaceTags = (id: string) => {
  return useQuery({
    queryKey: ['workspaces', id, 'tags'],
    queryFn: () => workspacesApi.listTags(id),
    enabled: !!id,
  });
};
