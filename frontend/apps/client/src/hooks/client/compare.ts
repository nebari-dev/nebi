// Owner: U4 (compare).
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { httpStatus } from '@/api/client/base';
import { compareApi } from '@/api/client/compare';
import { ERROR_BACKOFF_INTERVAL } from '@/hooks/useRemote';
import { queryKeys } from '@/lib/queryKeys';

/**
 * A 502 means a server-only side could not be fetched (server unreachable,
 * auth or server error). The page renders that state, so the query keeps
 * retrying on the error-backoff cadence until the server answers (AGENTS.md:
 * an errored query without an interval only refetches on remount). Other
 * errors (unknown version, invalid files) will not fix themselves.
 */
export const retryCompareWhileUnreachable = ({
  state,
}: {
  state: { status: string; error: unknown };
}) =>
  state.status === 'error' && httpStatus(state.error) === 502
    ? ERROR_BACKOFF_INTERVAL
    : false;

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
    // Keep the previous table on screen (dimmed) while a new pair loads.
    placeholderData: keepPreviousData,
    refetchInterval: retryCompareWhileUnreachable,
  });
