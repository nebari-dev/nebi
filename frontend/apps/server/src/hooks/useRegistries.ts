import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { registriesApi } from '@/api/registries';
import type {
  CreateRegistryRequest,
  ImportEnvironmentRequest,
  PublishRequest,
  UpdateRegistryRequest,
} from '@/types';

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

// Mutation hook for publishing project
export const usePublishProject = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      projectId,
      data,
    }: {
      projectId: string;
      data: PublishRequest;
    }) => registriesApi.publish(projectId, data),
    onSuccess: (_, variables) => {
      // Invalidate publications for this project and jobs list
      queryClient.invalidateQueries({
        queryKey: ['publications', variables.projectId],
      });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};

// Query hook for publish defaults (server-computed)
export const usePublishDefaults = (projectId: string, registryId?: string) => {
  return useQuery({
    queryKey: ['publish-defaults', projectId, registryId],
    queryFn: () => registriesApi.getPublishDefaults(projectId, registryId),
    enabled: !!projectId,
  });
};

// Query hook for project publications
export const usePublications = (projectId: string) => {
  return useQuery({
    queryKey: ['publications', projectId],
    queryFn: () => registriesApi.listPublications(projectId),
    enabled: !!projectId,
  });
};

// Mutation hook for updating publication visibility
export const useUpdatePublication = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      projectId,
      pubId,
      isPublic,
    }: {
      projectId: string;
      pubId: string;
      isPublic: boolean;
    }) => registriesApi.updatePublication(projectId, pubId, isPublic),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ['publications', variables.projectId],
      });
    },
  });
};

// Query hook for registry repositories (browse)
export const useRegistryRepositories = (
  registryId: string,
  search?: string,
) => {
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
    mutationFn: ({
      registryId,
      data,
    }: {
      registryId: string;
      data: ImportEnvironmentRequest;
    }) => registriesApi.importEnvironment(registryId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};
