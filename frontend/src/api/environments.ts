import { apiClient } from './client';
import type { Environment, CreateEnvironmentRequest, EnvironmentVersion, RollbackRequest, Job, EnvironmentTag } from '@/types';

export const environmentsApi = {
  list: async (): Promise<Environment[]> => {
    const { data } = await apiClient.get('/environments');
    return data;
  },

  get: async (id: string): Promise<Environment> => {
    const { data } = await apiClient.get(`/environments/${id}`);
    return data;
  },

  create: async (req: CreateEnvironmentRequest): Promise<Environment> => {
    const { data } = await apiClient.post('/environments', req);
    return data;
  },

  delete: async (id: string): Promise<void> => {
    await apiClient.delete(`/environments/${id}`);
  },

  getPixiToml: async (id: string): Promise<{ content: string }> => {
    const { data } = await apiClient.get(`/environments/${id}/pixi-toml`);
    return data;
  },

  // Version management
  listVersions: async (id: string): Promise<EnvironmentVersion[]> => {
    const { data } = await apiClient.get(`/environments/${id}/versions`);
    return data;
  },

  getVersion: async (id: string, versionNumber: number): Promise<EnvironmentVersion> => {
    const { data } = await apiClient.get(`/environments/${id}/versions/${versionNumber}`);
    return data;
  },

  downloadLockFile: async (id: string, versionNumber: number): Promise<string> => {
    const { data } = await apiClient.get(`/environments/${id}/versions/${versionNumber}/pixi-lock`, {
      responseType: 'text'
    });
    return data;
  },

  downloadManifest: async (id: string, versionNumber: number): Promise<string> => {
    const { data } = await apiClient.get(`/environments/${id}/versions/${versionNumber}/pixi-toml`, {
      responseType: 'text'
    });
    return data;
  },

  rollback: async (id: string, req: RollbackRequest): Promise<Job> => {
    const { data } = await apiClient.post(`/environments/${id}/rollback`, req);
    return data;
  },

  listTags: async (id: string): Promise<EnvironmentTag[]> => {
    const { data } = await apiClient.get(`/environments/${id}/tags`);
    return data;
  },
};
