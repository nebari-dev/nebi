import { useState } from 'react';
import { useRegistries, useDeleteRegistry } from '@/hooks/useRegistries';
import { CreateRegistryDialog } from '@/components/admin/CreateRegistryDialog';
import { EditRegistryDialog } from '@/components/admin/EditRegistryDialog';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Loader2, Pencil, Trash2, Star } from 'lucide-react';
import type { OCIRegistry } from '@/types';

export const RegistryManagement = () => {
  const { data: registries, isLoading } = useRegistries();
  const deleteRegistryMutation = useDeleteRegistry();

  const [editingRegistry, setEditingRegistry] = useState<OCIRegistry | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<{ id: string; name: string } | null>(null);
  const [error, setError] = useState('');

  const handleDelete = async () => {
    if (!deleteConfirm) return;

    setError('');
    try {
      await deleteRegistryMutation.mutateAsync(deleteConfirm.id);
      setDeleteConfirm(null);
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to delete registry. Please try again.';
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
          <p className="text-muted-foreground">Manage OCI registries for environment publishing</p>
        </div>
        <CreateRegistryDialog />
      </div>

      {error && (
        <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded">
          {error}
        </div>
      )}

      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="border-b bg-muted/50">
                <tr>
                  <th className="text-left p-4 font-medium">Name</th>
                  <th className="text-left p-4 font-medium">Registry URL</th>
                  <th className="text-left p-4 font-medium">Username</th>
                  <th className="text-left p-4 font-medium">Status</th>
                  <th className="text-left p-4 font-medium">Created</th>
                  <th className="text-right p-4 font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {registries?.map((registry) => (
                  <tr key={registry.id} className="border-b last:border-0 hover:bg-muted/50">
                    <td className="p-4 font-medium">
                      <div className="flex items-center gap-2">
                        {registry.name}
                        {registry.is_default && (
                          <Star className="h-4 w-4 fill-yellow-500 text-yellow-500" />
                        )}
                      </div>
                    </td>
                    <td className="p-4 text-sm text-muted-foreground font-mono">
                      {registry.url}
                    </td>
                    <td className="p-4 text-sm text-muted-foreground">
                      {registry.username || <span className="text-muted-foreground/50">—</span>}
                    </td>
                    <td className="p-4">
                      {registry.is_default ? (
                        <Badge className="bg-yellow-500/10 text-yellow-500 border-yellow-500/20">
                          Default
                        </Badge>
                      ) : (
                        <Badge variant="outline">Active</Badge>
                      )}
                    </td>
                    <td className="p-4 text-sm text-muted-foreground">
                      {new Date(registry.created_at).toLocaleDateString()}
                    </td>
                    <td className="p-4">
                      <div className="flex justify-end gap-2">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setEditingRegistry(registry)}
                          title="Edit Registry"
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
                          disabled={deleteRegistryMutation.isPending}
                          title="Delete Registry"
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {registries?.length === 0 && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">
            No registries configured. Add your first registry to start publishing environments.
          </p>
        </div>
      )}

      {editingRegistry && (
        <EditRegistryDialog
          registry={editingRegistry}
          open={!!editingRegistry}
          onOpenChange={(open) => !open && setEditingRegistry(null)}
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
