// Team-server sharing and catalog endpoints (internal/server/api).
import type {
  AccessList,
  CatalogProject,
  GrantAccessRequest,
  Principal,
  PrincipalRef,
  UpdateAccessRequest,
} from '@/types/sharing';
import { apiClient } from './client';

const accessPath = (projectId: string) =>
  `/projects/${encodeURIComponent(projectId)}/access`;

const principalPath = (projectId: string, p: PrincipalRef) =>
  `${accessPath(projectId)}/${p.kind}/${encodeURIComponent(p.id)}`;

export const sharingApi = {
  access: async (projectId: string): Promise<AccessList> =>
    (await apiClient.get(accessPath(projectId))).data,
  grant: async (
    projectId: string,
    req: GrantAccessRequest,
  ): Promise<AccessList> =>
    (await apiClient.post(accessPath(projectId), req)).data,
  update: async (
    projectId: string,
    p: PrincipalRef,
    req: UpdateAccessRequest,
  ): Promise<AccessList> =>
    (await apiClient.put(principalPath(projectId, p), req)).data,
  revoke: async (projectId: string, p: PrincipalRef): Promise<AccessList> =>
    (await apiClient.delete(principalPath(projectId, p))).data,
  principals: async (q: string): Promise<Principal[]> =>
    (await apiClient.get('/principals', { params: { q } })).data,
};

export const catalogApi = {
  list: async (): Promise<CatalogProject[]> =>
    (await apiClient.get('/catalog/projects')).data,
  /** Owner or admin only; the server removes the project in a job. */
  remove: async (projectId: string): Promise<void> => {
    await apiClient.delete(
      `/catalog/projects/${encodeURIComponent(projectId)}`,
    );
  },
};
