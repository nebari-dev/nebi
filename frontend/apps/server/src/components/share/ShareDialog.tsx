// Share dialog of the team server UI ("Share" / "Manage access" on the
// Server projects page). Same dialog as the client app, talking to the
// server's own access endpoints; the server decides who may change access.
import { useState } from 'react';
import {
  useAccess,
  useDebouncedValue,
  usePrincipals,
  useShareBackend,
} from '@/hooks/useSharing';
import { useAuthStore } from '@/store/authStore';
import { ShareDialogView } from './ShareDialogView';
import { applyAccessChange, errorMessage } from './shareModel';

export interface ShareDialogProps {
  projectId: string;
  projectName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const ShareDialog = ({
  projectId,
  projectName,
  open,
  onOpenChange,
}: ShareDialogProps) => {
  const access = useAccess(projectId, open);
  const username = useAuthStore((s) => s.user?.username);
  const [search, setSearch] = useState('');
  const debounced = useDebouncedValue(search);
  const principals = usePrincipals(open ? debounced : '');
  const { backend, busy } = useShareBackend(projectId);

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
      currentUsername={username}
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
