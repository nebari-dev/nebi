import type {
  Group,
  GroupMember,
  GroupWithMemberCount,
  ShareProjectWithGroupRequest,
} from '@/types/models';
import { apiClient } from './client';

export const groupsApi = {
  list: async (): Promise<GroupWithMemberCount[]> => {
    const r = await apiClient.get('/admin/groups');
    return r.data;
  },
  get: async (id: string): Promise<GroupWithMemberCount> => {
    const r = await apiClient.get(`/admin/groups/${id}`);
    return r.data;
  },

  listMembers: async (id: string): Promise<GroupMember[]> => {
    const r = await apiClient.get(`/admin/groups/${id}/members`);
    return r.data;
  },

  myGroups: async (): Promise<Group[]> => {
    const r = await apiClient.get('/groups/me');
    return r.data;
  },

  shareProject: async (
    projectId: string,
    body: ShareProjectWithGroupRequest,
  ): Promise<void> => {
    await apiClient.post(`/projects/${projectId}/share-group`, body);
  },
  unshareProject: async (projectId: string, groupId: string): Promise<void> => {
    await apiClient.delete(`/projects/${projectId}/share-group/${groupId}`);
  },
};
