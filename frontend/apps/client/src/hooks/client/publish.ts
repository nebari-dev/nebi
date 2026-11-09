// Owner: U7 (publish).
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { publishApi } from '@/api/client/publish';
import { queryKeys } from '@/lib/queryKeys';
import type {
  AddRegistryRequest,
  PublishRequest,
  RegistryPullRequest,
} from '@/types/contracts';

export const useRegistries = () =>
  useQuery({ queryKey: queryKeys.registries, queryFn: publishApi.registries });

export const useAddRegistry = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: AddRegistryRequest) => publishApi.addRegistry(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.registries }),
  });
};

export const useRemoveRegistry = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => publishApi.removeRegistry(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.registries }),
  });
};

export const useRegistryRepositories = (id: string, search: string) =>
  useQuery({
    queryKey: queryKeys.registryRepositories(id, search),
    queryFn: () => publishApi.repositories(id, search),
    enabled: !!id,
  });

export const useRegistryTags = (id: string, repository: string) =>
  useQuery({
    queryKey: queryKeys.registryTags(id, repository),
    queryFn: () => publishApi.tags(id, repository),
    enabled: !!id && !!repository,
  });

export const useRegistryPull = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      registryId,
      ...req
    }: RegistryPullRequest & { registryId: string }) =>
      publishApi.pull(registryId, req),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.projects }),
  });
};

export const usePublishDefaults = (projectId: string, n: number | undefined) =>
  useQuery({
    queryKey: queryKeys.publishDefaults(projectId, n ?? 0),
    queryFn: () => publishApi.defaults(projectId, n as number),
    enabled: !!projectId && !!n,
  });

export const usePublish = (projectId: string) => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ n, ...req }: PublishRequest & { n: number }) =>
      publishApi.publish(projectId, n, req),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: queryKeys.versions(projectId) }),
  });
};

export const useUnpublish = (projectId: string) => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (publicationId: string) =>
      publishApi.unpublish(projectId, publicationId),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: queryKeys.versions(projectId) }),
  });
};
