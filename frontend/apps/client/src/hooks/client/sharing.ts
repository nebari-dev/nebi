// Owner: U6 (sharing).
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { sharingApi } from '@/api/client/sharing';
import { queryKeys } from '@/lib/queryKeys';
import type {
  GrantAccessRequest,
  PrincipalRef,
  UpdateAccessRequest,
} from '@/types/contracts';

export const useAccess = (serverProjectId: string, enabled = true) =>
  useQuery({
    queryKey: queryKeys.access(serverProjectId),
    queryFn: () => sharingApi.access(serverProjectId),
    enabled: enabled && !!serverProjectId,
  });

export const usePrincipals = (q: string) =>
  useQuery({
    queryKey: queryKeys.principals(q),
    queryFn: () => sharingApi.principals(q),
    enabled: q.trim().length > 0,
  });

const useAccessMutation = <V>(
  serverProjectId: string,
  fn: (v: V) => ReturnType<typeof sharingApi.access>,
) => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: (list) =>
      qc.setQueryData(queryKeys.access(serverProjectId), list),
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
