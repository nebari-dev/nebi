import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { registriesApi } from '@/api/registries';
import type { CreateRegistryRequest, UpdateRegistryRequest, PublishRequest } from '@/types';

// Query hook for public registries (all authenticated users)
export const usePublicRegistries = () => {
  return useQuery({
    queryKey: ['registries', 'public'],
    queryFn: registriesApi.listPublic,
  });
};

// Query hook for admin registries list (with credentials)
export const useRegistries = () => {
  return useQuery({
    queryKey: ['registries', 'admin'],
    queryFn: registriesApi.list,
  });
};

// Query hook for single registry (admin)
export const useRegistry = (id: string) => {
  return useQuery({
    queryKey: ['registries', 'admin', id],
    queryFn: () => registriesApi.get(id),
    enabled: !!id,
  });
};

// Mutation hook for creating registry (admin)
export const useCreateRegistry = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateRegistryRequest) => registriesApi.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['registries'] });
    },
  });
};

// Mutation hook for updating registry (admin)
export const useUpdateRegistry = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateRegistryRequest }) =>
      registriesApi.update(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['registries'] });
    },
  });
};

// Mutation hook for deleting registry (admin)
export const useDeleteRegistry = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => registriesApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['registries'] });
    },
  });
};

// Mutation hook for publishing environment
export const usePublishEnvironment = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ environmentId, data }: { environmentId: string; data: PublishRequest }) =>
      registriesApi.publish(environmentId, data),
    onSuccess: (_, variables) => {
      // Invalidate publications for this environment and jobs list
      queryClient.invalidateQueries({ queryKey: ['publications', variables.environmentId] });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};

// Query hook for environment publications
export const usePublications = (environmentId: string) => {
  return useQuery({
    queryKey: ['publications', environmentId],
    queryFn: () => registriesApi.listPublications(environmentId),
    enabled: !!environmentId,
  });
};
