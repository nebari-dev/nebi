import type {
  CreateGroupRequest,
  Group,
  GroupMember,
  GroupWithMemberCount,
  ShareProjectWithGroupRequest,
  UpdateGroupRequest,
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
  create: async (data: CreateGroupRequest): Promise<Group> => {
    const r = await apiClient.post('/admin/groups', data);
    return r.data;
  },
  update: async (id: string, data: UpdateGroupRequest): Promise<Group> => {
    const r = await apiClient.patch(`/admin/groups/${id}`, data);
    return r.data;
  },
  remove: async (id: string): Promise<void> => {
    await apiClient.delete(`/admin/groups/${id}`);
  },

  listMembers: async (id: string): Promise<GroupMember[]> => {
    const r = await apiClient.get(`/admin/groups/${id}/members`);
    return r.data;
  },
  addMember: async (id: string, userId: string): Promise<void> => {
    await apiClient.post(`/admin/groups/${id}/members`, { user_id: userId });
  },
  removeMember: async (id: string, userId: string): Promise<void> => {
    await apiClient.delete(`/admin/groups/${id}/members/${userId}`);
  },

  grantAdmin: async (id: string): Promise<void> => {
    await apiClient.post(`/admin/groups/${id}/grant-admin`);
  },
  revokeAdmin: async (id: string): Promise<void> => {
    await apiClient.delete(`/admin/groups/${id}/grant-admin`);
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
