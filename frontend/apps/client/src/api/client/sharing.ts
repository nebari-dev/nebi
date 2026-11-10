// Owner: U6 (sharing).
import type {
  AccessList,
  GrantAccessRequest,
  Principal,
  PrincipalRef,
  UpdateAccessRequest,
} from '@/types/contracts';
import { apiClient, CLIENT } from './base';

const accessPath = (serverProjectId: string) =>
  `${CLIENT}/remote/projects/${serverProjectId}/access`;

export const sharingApi = {
  access: async (serverProjectId: string): Promise<AccessList> =>
    (await apiClient.get(accessPath(serverProjectId))).data,
  grant: async (
    serverProjectId: string,
    req: GrantAccessRequest,
  ): Promise<AccessList> =>
    (await apiClient.post(accessPath(serverProjectId), req)).data,
  update: async (
    serverProjectId: string,
    p: PrincipalRef,
    req: UpdateAccessRequest,
  ): Promise<AccessList> =>
    (
      await apiClient.put(
        `${accessPath(serverProjectId)}/${p.kind}/${p.id}`,
        req,
      )
    ).data,
  revoke: async (
    serverProjectId: string,
    p: PrincipalRef,
  ): Promise<AccessList> =>
    (await apiClient.delete(`${accessPath(serverProjectId)}/${p.kind}/${p.id}`))
      .data,
  principals: async (q: string): Promise<Principal[]> =>
    (await apiClient.get(`${CLIENT}/remote/principals`, { params: { q } }))
      .data,
};
