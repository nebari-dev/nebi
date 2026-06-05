import type {
  CreateGroupRequest,
  Group,
  GroupMember,
  GroupWithMemberCount,
  ShareWorkspaceWithGroupRequest,
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

  shareWorkspace: async (
    workspaceId: string,
    body: ShareWorkspaceWithGroupRequest,
  ): Promise<void> => {
    await apiClient.post(`/workspaces/${workspaceId}/share-group`, body);
  },
  unshareWorkspace: async (
    workspaceId: string,
    groupId: string,
  ): Promise<void> => {
    await apiClient.delete(`/workspaces/${workspaceId}/share-group/${groupId}`);
  },
};
