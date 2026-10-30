import { useQuery } from '@tanstack/react-query';
import { groupsApi } from '@/api/groups';

const groupsKey = ['groups'] as const;

export const useGroups = (enabled = true) =>
  useQuery({ queryKey: groupsKey, queryFn: groupsApi.list, enabled });

export const useGroup = (id: string | undefined) =>
  useQuery({
    queryKey: ['group', id],
    queryFn: () => {
      if (!id) throw new Error('Group ID is required');
      return groupsApi.get(id);
    },
    enabled: !!id,
  });

export const useGroupMembers = (id: string | undefined) =>
  useQuery({
    queryKey: ['group', id, 'members'],
    queryFn: () => {
      if (!id) throw new Error('Group ID is required');
      return groupsApi.listMembers(id);
    },
    enabled: !!id,
  });

export const useMyGroups = (enabled = true) =>
  useQuery({
    queryKey: ['groups', 'me'],
    queryFn: groupsApi.myGroups,
    enabled,
  });
