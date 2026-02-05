import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { environmentsApi } from '@/api/environments';
import type { CreateEnvironmentRequest } from '@/types';

export const useEnvironments = () => {
  return useQuery({
    queryKey: ['environments'],
    queryFn: environmentsApi.list,
    refetchInterval: 2000, // Poll every 2 seconds for status updates
  });
};

export const useEnvironment = (id: string) => {
  return useQuery({
    queryKey: ['environments', id],
    queryFn: () => environmentsApi.get(id),
    enabled: !!id,
    refetchInterval: 2000,
  });
};

export const useCreateEnvironment = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateEnvironmentRequest) => environmentsApi.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
};

export const useDeleteEnvironment = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => environmentsApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
};

export const useEnvironmentTags = (id: string) => {
  return useQuery({
    queryKey: ['environments', id, 'tags'],
    queryFn: () => environmentsApi.listTags(id),
    enabled: !!id,
  });
};
