import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { registriesApi } from '@/api/registries';
import type { CreateRegistryRequest, UpdateRegistryRequest, PublishRequest, ImportEnvironmentRequest } from '@/types';

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

// Mutation hook for publishing workspace
export const usePublishWorkspace = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ workspaceId, data }: { workspaceId: string; data: PublishRequest }) =>
      registriesApi.publish(workspaceId, data),
    onSuccess: (_, variables) => {
      // Invalidate publications for this workspace and jobs list
      queryClient.invalidateQueries({ queryKey: ['publications', variables.workspaceId] });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};

// Query hook for publish defaults (server-computed)
export const usePublishDefaults = (workspaceId: string) => {
  return useQuery({
    queryKey: ['publish-defaults', workspaceId],
    queryFn: () => registriesApi.getPublishDefaults(workspaceId),
    enabled: !!workspaceId,
  });
};

// Query hook for workspace publications
export const usePublications = (workspaceId: string) => {
  return useQuery({
    queryKey: ['publications', workspaceId],
    queryFn: () => registriesApi.listPublications(workspaceId),
    enabled: !!workspaceId,
  });
};

// Query hook for registry repositories (browse)
export const useRegistryRepositories = (registryId: string, search?: string) => {
  return useQuery({
    queryKey: ['registries', registryId, 'repositories', search],
    queryFn: () => registriesApi.listRepositories(registryId, search),
    enabled: !!registryId,
  });
};

// Query hook for repository tags (browse)
export const useRepositoryTags = (registryId: string, repo: string) => {
  return useQuery({
    queryKey: ['registries', registryId, 'tags', repo],
    queryFn: () => registriesApi.listTags(registryId, repo),
    enabled: !!registryId && !!repo,
  });
};

// Mutation hook for importing an environment from a registry
export const useImportEnvironment = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ registryId, data }: { registryId: string; data: ImportEnvironmentRequest }) =>
      registriesApi.importEnvironment(registryId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};
