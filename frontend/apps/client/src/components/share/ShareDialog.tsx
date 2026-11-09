// Owner: U6 (sharing). Opened from the project detail page (U2, "Share")
// and the Server page row menu (U5, "Manage access"). Props are a
// cross-unit contract (CONTRACTS.md). Calls go through the client API,
// which proxies them to the connected team server; the server decides who
// may see and change access.
import { useState } from 'react';
import { useServerConnection } from '@/hooks/client/remote';
import {
  useAccess,
  useDebouncedValue,
  usePrincipals,
  useShareBackend,
} from '@/hooks/client/sharing';
import { ShareDialogView } from './ShareDialogView';
import { applyAccessChange, errorMessage } from './shareModel';

export interface ShareDialogProps {
  serverProjectId: string;
  projectName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const ShareDialog = ({
  serverProjectId,
  projectName,
  open,
  onOpenChange,
}: ShareDialogProps) => {
  const access = useAccess(serverProjectId, open);
  const { data: connection } = useServerConnection();
  const [search, setSearch] = useState('');
  const debounced = useDebouncedValue(search);
  const principals = usePrincipals(open ? debounced : '');
  const { backend, busy } = useShareBackend(serverProjectId);

  return (
    <ShareDialogView
      projectName={projectName}
      open={open}
      onOpenChange={onOpenChange}
      access={access.data}
      loading={access.isPending}
      loadError={
        access.isError
          ? errorMessage(access.error, 'Couldn’t load who has access.')
          : null
      }
      onRetry={() => access.refetch()}
      currentUsername={connection?.username}
      onSearchChange={setSearch}
      candidates={debounced.trim() ? (principals.data ?? []) : []}
      searching={principals.isFetching || search !== debounced}
      busy={busy}
      onShare={(p, role) =>
        applyAccessChange(backend, projectName, p, role, null)
      }
      onChangeRole={(e, role) =>
        applyAccessChange(
          backend,
          projectName,
          e.principal,
          role,
          e.role === 'owner' ? null : e.role,
        )
      }
      onRemove={(e) =>
        applyAccessChange(
          backend,
          projectName,
          e.principal,
          null,
          e.role === 'owner' ? null : e.role,
        )
      }
    />
  );
};
