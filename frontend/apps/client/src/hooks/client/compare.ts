// Owner: U4 (compare).
import { useQuery } from '@tanstack/react-query';
import { compareApi } from '@/api/client/compare';
import { queryKeys } from '@/lib/queryKeys';

export const useCompare = (
  projectId: string,
  base: number | undefined,
  target: number | undefined,
) =>
  useQuery({
    queryKey: queryKeys.compare(projectId, base ?? 0, target ?? 0),
    queryFn: () =>
      compareApi.compare(projectId, base as number, target as number),
    enabled: !!projectId && !!base && !!target,
  });
