// Owner: U3 (newversion).
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { newVersionApi } from '@/api/client/newversion';
import { queryKeys } from '@/lib/queryKeys';
import type { PreviewChangesRequest } from '@/types/client/newversion';
import type { CreateVersionRequest } from '@/types/contracts';

export const useCreateVersion = (projectId: string) => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: CreateVersionRequest) =>
      newVersionApi.create(projectId, req),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: queryKeys.project(projectId) }),
  });
};

/** One-off preview (e.g. before submitting). */
export const usePreviewChanges = (projectId: string) =>
  useMutation({
    mutationFn: (req: PreviewChangesRequest) =>
      newVersionApi.preview(projectId, req),
  });

/**
 * Starting point of a new version from `from` (undefined = latest). Kept
 * under the version's query key so project invalidation refreshes it.
 */
export const useVersionDraft = (projectId: string, from?: number) =>
  useQuery({
    queryKey: [
      ...queryKeys.versions(projectId),
      'draft',
      from ?? 'latest',
    ] as const,
    queryFn: () => newVersionApi.draft(projectId, from ?? 'latest'),
    enabled: !!projectId,
    // The page keeps its own edited copy; never refetch underneath it.
    staleTime: Number.POSITIVE_INFINITY,
    refetchOnWindowFocus: false,
    retry: false,
  });

/**
 * Live "Summary of changes" for a draft. Pass null while there is nothing
 * to compare. The previous summary stays visible while a new one loads.
 */
export const useChangesSummary = (
  projectId: string,
  req: PreviewChangesRequest | null,
) =>
  useQuery({
    queryKey: [
      ...queryKeys.version(projectId, req?.base_version ?? 0),
      'preview',
      req,
    ] as const,
    queryFn: () =>
      newVersionApi.preview(projectId, req as PreviewChangesRequest),
    enabled: !!projectId && !!req,
    placeholderData: keepPreviousData,
    retry: false,
    staleTime: 30_000,
  });
