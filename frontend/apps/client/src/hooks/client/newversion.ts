// Owner: U3 (newversion).
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { newVersionApi } from '@/api/client/newversion';
import { queryKeys } from '@/lib/queryKeys';
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

export const usePreviewChanges = (projectId: string) =>
  useMutation({
    mutationFn: ({
      baseVersion,
      pixiToml,
    }: {
      baseVersion: number;
      pixiToml: string;
    }) => newVersionApi.preview(projectId, baseVersion, pixiToml),
  });
