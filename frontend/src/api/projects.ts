import type {
  CreateProjectRequest,
  Job,
  Project,
  ProjectTag,
  ProjectVersion,
  RollbackRequest,
} from '@/types';
import { apiClient } from './client';

export const projectsApi = {
  list: async (): Promise<Project[]> => {
    const { data } = await apiClient.get('/projects');
    return data;
  },

  get: async (id: string): Promise<Project> => {
    const { data } = await apiClient.get(`/projects/${id}`);
    return data;
  },

  create: async (req: CreateProjectRequest): Promise<Project> => {
    const { data } = await apiClient.post('/projects', req);
    return data;
  },

  delete: async (id: string): Promise<void> => {
    await apiClient.delete(`/projects/${id}`);
  },

  getPixiToml: async (id: string): Promise<{ content: string }> => {
    const { data } = await apiClient.get(`/projects/${id}/pixi-toml`);
    return data;
  },

  // Version management
  listVersions: async (id: string): Promise<ProjectVersion[]> => {
    const { data } = await apiClient.get(`/projects/${id}/versions`);
    return data;
  },

  getVersion: async (
    id: string,
    versionNumber: number,
  ): Promise<ProjectVersion> => {
    const { data } = await apiClient.get(
      `/projects/${id}/versions/${versionNumber}`,
    );
    return data;
  },

  downloadLockFile: async (
    id: string,
    versionNumber: number,
  ): Promise<string> => {
    const { data } = await apiClient.get(
      `/projects/${id}/versions/${versionNumber}/pixi-lock`,
      {
        responseType: 'text',
      },
    );
    return data;
  },

  downloadManifest: async (
    id: string,
    versionNumber: number,
  ): Promise<string> => {
    const { data } = await apiClient.get(
      `/projects/${id}/versions/${versionNumber}/pixi-toml`,
      {
        responseType: 'text',
      },
    );
    return data;
  },

  rollback: async (id: string, req: RollbackRequest): Promise<Job> => {
    const { data } = await apiClient.post(`/projects/${id}/rollback`, req);
    return data;
  },

  listTags: async (id: string): Promise<ProjectTag[]> => {
    const { data } = await apiClient.get(`/projects/${id}/tags`);
    return data;
  },

  savePixiToml: async (id: string, content: string): Promise<void> => {
    await apiClient.put(`/projects/${id}/pixi-toml`, { content });
  },

  solveProject: async (id: string): Promise<Job> => {
    const { data } = await apiClient.post(`/projects/${id}/solve`);
    return data;
  },

  install: async (id: string): Promise<Job> => {
    const { data } = await apiClient.post(`/projects/${id}/install`);
    return data;
  },

  uninstall: async (id: string): Promise<Job> => {
    const { data } = await apiClient.post(`/projects/${id}/uninstall`);
    return data;
  },
};
