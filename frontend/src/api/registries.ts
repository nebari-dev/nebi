import { apiClient } from './client';
import type { OCIRegistry, CreateRegistryRequest, UpdateRegistryRequest, Publication, PublishDefaults, PublishRequest, Job, RegistryRepository, RegistryTag, ImportEnvironmentRequest, Workspace } from '@/types';

export const registriesApi = {
  // Public endpoints (for all authenticated users)
  listPublic: async (): Promise<OCIRegistry[]> => {
    const { data } = await apiClient.get('/registries');
    return data;
  },

  // Admin endpoints (require admin role)
  list: async (): Promise<OCIRegistry[]> => {
    const { data } = await apiClient.get('/admin/registries');
    return data;
  },

  get: async (id: string): Promise<OCIRegistry> => {
    const { data } = await apiClient.get(`/admin/registries/${id}`);
    return data;
  },

  create: async (req: CreateRegistryRequest): Promise<OCIRegistry> => {
    const { data } = await apiClient.post('/admin/registries', req);
    return data;
  },

  update: async (id: string, req: UpdateRegistryRequest): Promise<OCIRegistry> => {
    const { data } = await apiClient.put(`/admin/registries/${id}`, req);
    return data;
  },

  delete: async (id: string): Promise<void> => {
    await apiClient.delete(`/admin/registries/${id}`);
  },

  // Publishing endpoints (require write permission on workspace)
  getPublishDefaults: async (workspaceId: string): Promise<PublishDefaults> => {
    const { data } = await apiClient.get(`/workspaces/${workspaceId}/publish-defaults`);
    return data;
  },

  publish: async (workspaceId: string, req: PublishRequest): Promise<Job> => {
    const { data } = await apiClient.post(`/workspaces/${workspaceId}/publish`, req);
    return data;
  },

  listPublications: async (workspaceId: string): Promise<Publication[]> => {
    const { data } = await apiClient.get(`/workspaces/${workspaceId}/publications`);
    return data;
  },

  // Browse endpoints (for all authenticated users)
  listRepositories: async (registryId: string, search?: string): Promise<{ repositories: RegistryRepository[]; fallback: boolean }> => {
    const params = search ? { search } : {};
    const { data } = await apiClient.get(`/registries/${registryId}/repositories`, { params });
    return data;
  },

  listTags: async (registryId: string, repo: string): Promise<{ tags: RegistryTag[] }> => {
    const { data } = await apiClient.get(`/registries/${registryId}/tags`, { params: { repo } });
    return data;
  },

  importEnvironment: async (registryId: string, req: ImportEnvironmentRequest): Promise<Workspace> => {
    const { data } = await apiClient.post(`/registries/${registryId}/import`, req);
    return data;
  },
};
