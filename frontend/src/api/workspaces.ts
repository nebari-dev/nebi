import { apiClient } from './client';
import type { Workspace, CreateWorkspaceRequest, WorkspaceVersion, RollbackRequest, Job, WorkspaceTag } from '@/types';

export const workspacesApi = {
  list: async (): Promise<Workspace[]> => {
    const { data } = await apiClient.get('/workspaces');
    return data;
  },

  get: async (id: string): Promise<Workspace> => {
    const { data } = await apiClient.get(`/workspaces/${id}`);
    return data;
  },

  create: async (req: CreateWorkspaceRequest): Promise<Workspace> => {
    const { data } = await apiClient.post('/workspaces', req);
    return data;
  },

  delete: async (id: string): Promise<void> => {
    await apiClient.delete(`/workspaces/${id}`);
  },

  getPixiToml: async (id: string): Promise<{ content: string }> => {
    const { data } = await apiClient.get(`/workspaces/${id}/pixi-toml`);
    return data;
  },

  // Version management
  listVersions: async (id: string): Promise<WorkspaceVersion[]> => {
    const { data } = await apiClient.get(`/workspaces/${id}/versions`);
    return data;
  },

  getVersion: async (id: string, versionNumber: number): Promise<WorkspaceVersion> => {
    const { data } = await apiClient.get(`/workspaces/${id}/versions/${versionNumber}`);
    return data;
  },

  downloadLockFile: async (id: string, versionNumber: number): Promise<string> => {
    const { data } = await apiClient.get(`/workspaces/${id}/versions/${versionNumber}/pixi-lock`, {
      responseType: 'text'
    });
    return data;
  },

  downloadManifest: async (id: string, versionNumber: number): Promise<string> => {
    const { data } = await apiClient.get(`/workspaces/${id}/versions/${versionNumber}/pixi-toml`, {
      responseType: 'text'
    });
    return data;
  },

  rollback: async (id: string, req: RollbackRequest): Promise<Job> => {
    const { data } = await apiClient.post(`/workspaces/${id}/rollback`, req);
    return data;
  },

  listTags: async (id: string): Promise<WorkspaceTag[]> => {
    const { data } = await apiClient.get(`/workspaces/${id}/tags`);
    return data;
  },
};
