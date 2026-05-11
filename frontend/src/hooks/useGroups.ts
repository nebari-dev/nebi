import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { groupsApi } from '@/api/groups';
import type { CreateGroupRequest, UpdateGroupRequest } from '@/types/models';

const groupsKey = ['groups'] as const;

export const useGroups = () =>
  useQuery({ queryKey: groupsKey, queryFn: groupsApi.list });

export const useGroup = (id: string | undefined) =>
  useQuery({
    queryKey: ['group', id],
    queryFn: () => groupsApi.get(id!),
    enabled: !!id,
  });

export const useGroupMembers = (id: string | undefined) =>
  useQuery({
    queryKey: ['group', id, 'members'],
    queryFn: () => groupsApi.listMembers(id!),
    enabled: !!id,
  });

export const useMyGroups = (enabled = true) =>
  useQuery({ queryKey: ['groups', 'me'], queryFn: groupsApi.myGroups, enabled });

export const useCreateGroup = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateGroupRequest) => groupsApi.create(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: groupsKey }),
  });
};

export const useUpdateGroup = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateGroupRequest }) =>
      groupsApi.update(id, data),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: groupsKey });
      qc.invalidateQueries({ queryKey: ['group', id] });
    },
  });
};

export const useDeleteGroup = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => groupsApi.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: groupsKey }),
  });
};

export const useAddGroupMember = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, userId }: { id: string; userId: string }) =>
      groupsApi.addMember(id, userId),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: ['group', id, 'members'] });
      qc.invalidateQueries({ queryKey: groupsKey });
    },
  });
};

export const useRemoveGroupMember = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, userId }: { id: string; userId: string }) =>
      groupsApi.removeMember(id, userId),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: ['group', id, 'members'] });
      qc.invalidateQueries({ queryKey: groupsKey });
    },
  });
};
