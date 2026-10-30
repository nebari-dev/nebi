import type {
  CreateRegistryRequest,
  ImportEnvironmentRequest,
  Job,
  OCIRegistry,
  Project,
  Publication,
  PublishDefaults,
  PublishRequest,
  RegistryRepository,
  RegistryTag,
  UpdateRegistryRequest,
} from '@/types';
import { apiClient } from './client';

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

  update: async (
    id: string,
    req: UpdateRegistryRequest,
  ): Promise<OCIRegistry> => {
    const { data } = await apiClient.put(`/admin/registries/${id}`, req);
    return data;
  },

  delete: async (id: string): Promise<void> => {
    await apiClient.delete(`/admin/registries/${id}`);
  },

  // Publishing endpoints (require write permission on project)
  getPublishDefaults: async (
    projectId: string,
    registryId?: string,
  ): Promise<PublishDefaults> => {
    const { data } = await apiClient.get(
      `/projects/${projectId}/publish-defaults`,
      {
        params: registryId ? { registry_id: registryId } : undefined,
      },
    );
    return data;
  },

  publish: async (projectId: string, req: PublishRequest): Promise<Job> => {
    const { data } = await apiClient.post(
      `/projects/${projectId}/publish`,
      req,
    );
    return data;
  },

  listPublications: async (projectId: string): Promise<Publication[]> => {
    const { data } = await apiClient.get(`/projects/${projectId}/publications`);
    return data;
  },

  updatePublication: async (
    projectId: string,
    pubId: string,
    isPublic: boolean,
  ): Promise<Publication> => {
    const { data } = await apiClient.patch(
      `/projects/${projectId}/publications/${pubId}`,
      { is_public: isPublic },
    );
    return data;
  },

  // Browse endpoints (for all authenticated users)
  listRepositories: async (
    registryId: string,
    search?: string,
  ): Promise<{ repositories: RegistryRepository[]; fallback: boolean }> => {
    const params = search ? { search } : {};
    const { data } = await apiClient.get(
      `/registries/${registryId}/repositories`,
      { params },
    );
    return data;
  },

  listTags: async (
    registryId: string,
    repo: string,
  ): Promise<{ tags: RegistryTag[] }> => {
    const { data } = await apiClient.get(`/registries/${registryId}/tags`, {
      params: { repo },
    });
    return data;
  },

  importEnvironment: async (
    registryId: string,
    req: ImportEnvironmentRequest,
  ): Promise<Project> => {
    const { data } = await apiClient.post(
      `/registries/${registryId}/import`,
      req,
    );
    return data;
  },
};
