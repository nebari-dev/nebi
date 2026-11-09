// Server projects (catalog), sharing and delete-from-server for the team
// server UI.
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { catalogApi, sharingApi } from '@/api/sharing';
import type { ShareBackend } from '@/components/share/shareModel';
import type {
  AccessList,
  GrantAccessRequest,
  PrincipalRef,
  UpdateAccessRequest,
} from '@/types/sharing';

export const sharingKeys = {
  catalog: ['server', 'catalog'] as const,
  access: (projectId: string) => ['server', 'access', projectId] as const,
  principals: (q: string) => ['server', 'principals', q] as const,
};

/** Server projects visible to the signed-in user. */
export const useCatalogProjects = () =>
  useQuery({
    queryKey: sharingKeys.catalog,
    queryFn: catalogApi.list,
  });

export const useDeleteServerProject = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (projectId: string) => catalogApi.remove(projectId),
    onSuccess: (_, projectId) => {
      qc.removeQueries({ queryKey: sharingKeys.access(projectId) });
      qc.invalidateQueries({ queryKey: sharingKeys.catalog });
    },
  });
};

export const useAccess = (projectId: string, enabled = true) =>
  useQuery({
    queryKey: sharingKeys.access(projectId),
    queryFn: () => sharingApi.access(projectId),
    enabled: enabled && !!projectId,
  });

export const usePrincipals = (q: string) =>
  useQuery({
    queryKey: sharingKeys.principals(q.trim()),
    queryFn: () => sharingApi.principals(q.trim()),
    enabled: q.trim().length > 0,
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  });

/** value, updated after it has not changed for delayMs. */
export const useDebouncedValue = <T>(value: T, delayMs = 250): T => {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = window.setTimeout(() => setDebounced(value), delayMs);
    return () => window.clearTimeout(t);
  }, [value, delayMs]);
  return debounced;
};

const useAccessMutation = <V>(
  projectId: string,
  fn: (v: V) => Promise<AccessList>,
) => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: (list) => qc.setQueryData(sharingKeys.access(projectId), list),
  });
};

/** The share dialog calls, each updating the cached access list. */
export const useShareBackend = (projectId: string) => {
  const grant = useAccessMutation(projectId, (req: GrantAccessRequest) =>
    sharingApi.grant(projectId, req),
  );
  const update = useAccessMutation(
    projectId,
    ({
      principal,
      ...req
    }: UpdateAccessRequest & { principal: PrincipalRef }) =>
      sharingApi.update(projectId, principal, req),
  );
  const revoke = useAccessMutation(projectId, (p: PrincipalRef) =>
    sharingApi.revoke(projectId, p),
  );
  const backend: ShareBackend = useMemo(
    () => ({
      grant: (p, role) => grant.mutateAsync({ principals: [p], role }),
      update: (p, role) => update.mutateAsync({ principal: p, role }),
      revoke: (p) => revoke.mutateAsync(p),
    }),
    [grant.mutateAsync, update.mutateAsync, revoke.mutateAsync],
  );
  return {
    backend,
    busy: grant.isPending || update.isPending || revoke.isPending,
  };
};
