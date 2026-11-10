// Owner: U6 (sharing). Share dialog data, proxied by the client API to the
// connected team server (/client/remote/...).
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { sharingApi } from '@/api/client/sharing';
import type { ShareBackend } from '@/components/share/shareModel';
import { retryWhileUnreachable } from '@/hooks/useRemote';
import { queryKeys } from '@/lib/queryKeys';
import type {
  AccessList,
  GrantAccessRequest,
  PrincipalRef,
  UpdateAccessRequest,
} from '@/types/contracts';

/**
 * Who has access to a server project. The dialog renders its load error, so
 * an errored query keeps retrying on its own while it is shown (AGENTS.md:
 * refetchOnWindowFocus is off app-wide).
 */
export const useAccess = (serverProjectId: string, enabled = true) =>
  useQuery({
    queryKey: queryKeys.access(serverProjectId),
    queryFn: () => sharingApi.access(serverProjectId),
    enabled: enabled && !!serverProjectId,
    refetchInterval: retryWhileUnreachable,
  });

/** People and teams matching q (server search; empty q fetches nothing). */
export const usePrincipals = (q: string) =>
  useQuery({
    queryKey: queryKeys.principals(q.trim()),
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
  serverProjectId: string,
  fn: (v: V) => Promise<AccessList>,
) => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: (list) => {
      qc.setQueryData(queryKeys.access(serverProjectId), list);
      // A server project's role/visibility may show on the Server page.
      qc.invalidateQueries({ queryKey: ['client', 'remote', 'projects'] });
    },
  });
};

export const useGrantAccess = (serverProjectId: string) =>
  useAccessMutation(serverProjectId, (req: GrantAccessRequest) =>
    sharingApi.grant(serverProjectId, req),
  );

export const useUpdateAccess = (serverProjectId: string) =>
  useAccessMutation(
    serverProjectId,
    ({
      principal,
      ...req
    }: UpdateAccessRequest & { principal: PrincipalRef }) =>
      sharingApi.update(serverProjectId, principal, req),
  );

export const useRevokeAccess = (serverProjectId: string) =>
  useAccessMutation(serverProjectId, (principal: PrincipalRef) =>
    sharingApi.revoke(serverProjectId, principal),
  );

/**
 * The calls the share dialog makes, each updating the cached access list
 * (also when invoked from a toast's Undo after the dialog closed).
 */
export const useShareBackend = (serverProjectId: string) => {
  const grant = useGrantAccess(serverProjectId);
  const update = useUpdateAccess(serverProjectId);
  const revoke = useRevokeAccess(serverProjectId);
  const backend: ShareBackend = useMemo(
    () => ({
      grant: (p, role) => grant.mutateAsync({ principals: [p], role }),
      update: (p, role) => update.mutateAsync({ principal: p, role }),
      revoke: (p) => revoke.mutateAsync(p),
    }),
    [grant.mutateAsync, update.mutateAsync, revoke.mutateAsync],
  );
  const busy = grant.isPending || update.isPending || revoke.isPending;
  return { backend, busy };
};
