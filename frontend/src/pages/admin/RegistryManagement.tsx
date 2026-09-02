import { Loader2, Pencil, Star, Trash2 } from 'lucide-react';
import { useMemo, useState } from 'react';
import { CreateRegistryDialog } from '@/components/admin/CreateRegistryDialog';
import { EditRegistryDialog } from '@/components/admin/EditRegistryDialog';
import { RemoteUnreachableBanner } from '@/components/remote/RemoteUnreachableBanner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useDeleteRegistry, useRegistries } from '@/hooks/useRegistries';
import {
  useDeleteRemoteRegistry,
  useRemoteAdminRegistries,
  useRemoteView,
} from '@/hooks/useRemote';
import type { OCIRegistry } from '@/types';

export const RegistryManagement = () => {
  const { data: registries, isLoading: registriesLoading } = useRegistries();
  const deleteLocalMutation = useDeleteRegistry();
  const deleteRemoteMutation = useDeleteRemoteRegistry();

  // View mode support
  const { viewMode, isRemoteConnected, isRemoteView } = useRemoteView();

  // Delete/edit must target the same server the displayed rows came from.
  const deleteRegistryMutation = isRemoteView
    ? deleteRemoteMutation
    : deleteLocalMutation;
  const {
    data: remoteRegistries,
    isFirstLoad: remoteFirstLoad,
    isUnreachable: remoteIsUnreachable,
  } = useRemoteAdminRegistries(isRemoteView);

  // Show registries based on view mode
  const displayedRegistries = useMemo(() => {
    if (!isRemoteConnected) {
      return registries || [];
    }
    if (viewMode === 'local') {
      return registries || [];
    } else {
      return remoteRegistries || [];
    }
  }, [registries, remoteRegistries, isRemoteConnected, viewMode]);

  const remoteUnreachable = isRemoteView && remoteIsUnreachable;
  // Full-page spinner only until the remote list first resolves or errors
  // (see isFirstLoad in useRemote.ts).
  const isLoading = registriesLoading || (isRemoteView && remoteFirstLoad);

  const [editingRegistry, setEditingRegistry] = useState<OCIRegistry | null>(
    null,
  );
  const [deleteConfirm, setDeleteConfirm] = useState<{
    id: string;
    name: string;
  } | null>(null);
  const [error, setError] = useState('');

  const handleDelete = async () => {
    if (!deleteConfirm) return;

    setError('');
    try {
      await deleteRegistryMutation.mutateAsync(deleteConfirm.id);
      setDeleteConfirm(null);
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage =
        error?.response?.data?.error ||
        'Failed to delete registry. Please try again.';
      setError(errorMessage);
      setDeleteConfirm(null);
    }
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold">OCI Registry Management</h1>
          <p className="text-muted-foreground">
            Manage OCI registries for workspace publishing
          </p>
        </div>
        <CreateRegistryDialog isRemote={isRemoteView} />
      </div>

      {error && (
        <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded">
          {error}
        </div>
      )}

      {remoteUnreachable && <RemoteUnreachableBanner />}

      <Table aria-label="OCI registries">
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Registry URL</TableHead>
            <TableHead>Username</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Created</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {displayedRegistries.map((registry) => (
            <TableRow key={registry.id}>
              <TableCell className="font-medium">
                <div className="flex items-center gap-2">
                  {registry.name}
                  {registry.is_default && (
                    <Star className="h-4 w-4 fill-yellow-500 text-yellow-500" />
                  )}
                </div>
              </TableCell>
              <TableCell className="text-muted-foreground font-mono">
                {registry.url}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {registry.username || (
                  <span className="text-muted-foreground/50">—</span>
                )}
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-2">
                  {registry.is_default ? (
                    <Badge className="bg-yellow-100 text-yellow-800 border-yellow-300">
                      Default
                    </Badge>
                  ) : (
                    <Badge variant="outline">Active</Badge>
                  )}
                  {registry.config_managed && (
                    <Badge
                      variant="outline"
                      title="Defined in server configuration (config.yaml). Edit or remove it there."
                    >
                      Managed
                    </Badge>
                  )}
                  {registry.restricted && (
                    <Badge
                      variant="outline"
                      title="Only groups granted access can use this registry."
                    >
                      Restricted
                    </Badge>
                  )}
                </div>
              </TableCell>
              <TableCell className="text-muted-foreground">
                {new Date(registry.created_at).toLocaleDateString()}
              </TableCell>
              <TableCell>
                <div className="flex justify-end gap-2">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setEditingRegistry(registry)}
                    aria-label={`Edit ${registry.name}`}
                    disabled={registry.config_managed}
                    title={
                      registry.config_managed
                        ? 'Managed by server configuration - edit registry'
                        : 'Edit Registry'
                    }
                  >
                    <Pencil className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      setDeleteConfirm({
                        id: registry.id,
                        name: registry.name,
                      })
                    }
                    aria-label={`Delete ${registry.name}`}
                    disabled={
                      deleteRegistryMutation.isPending ||
                      registry.config_managed
                    }
                    title={
                      registry.config_managed
                        ? 'Managed by server configuration - delete registry'
                        : 'Delete Registry'
                    }
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      {displayedRegistries.length === 0 && !remoteUnreachable && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">
            No registries configured. Add your first registry to start
            publishing workspaces.
          </p>
        </div>
      )}

      {editingRegistry && (
        <EditRegistryDialog
          registry={editingRegistry}
          open={!!editingRegistry}
          onOpenChange={(open) => !open && setEditingRegistry(null)}
          isRemote={isRemoteView}
        />
      )}

      <ConfirmDialog
        open={!!deleteConfirm}
        onOpenChange={(open) => !open && setDeleteConfirm(null)}
        onConfirm={handleDelete}
        title="Delete Registry"
        description={`Are you sure you want to delete ${deleteConfirm?.name}? Any existing publications using this registry will still be accessible, but you won't be able to publish new versions to it.`}
        confirmText="Delete"
        cancelText="Cancel"
        variant="destructive"
      />
    </div>
  );
};
