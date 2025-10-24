import { useState } from 'react';
import { useUsers, useToggleAdmin, useDeleteUser } from '@/hooks/useAdmin';
import { useAuthStore } from '@/store/authStore';
import { CreateUserDialog } from '@/components/admin/CreateUserDialog';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Loader2, Shield, ShieldOff, Trash2 } from 'lucide-react';

export const UserManagement = () => {
  const { data: users, isLoading } = useUsers();
  const toggleAdminMutation = useToggleAdmin();
  const deleteUserMutation = useDeleteUser();
  const currentUser = useAuthStore((state) => state.user);

  const [confirmAction, setConfirmAction] = useState<{
    type: 'toggle' | 'delete';
    userId: string;
    username?: string;
    currentIsAdmin?: boolean;
  } | null>(null);

  const handleConfirmAction = async () => {
    if (!confirmAction) return;

    if (confirmAction.type === 'toggle') {
      await toggleAdminMutation.mutateAsync(confirmAction.userId);
    } else if (confirmAction.type === 'delete') {
      await deleteUserMutation.mutateAsync(confirmAction.userId);
    }

    setConfirmAction(null);
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
          <h1 className="text-3xl font-bold">User Management</h1>
          <p className="text-muted-foreground">Manage user accounts and permissions</p>
        </div>
        <CreateUserDialog />
      </div>

      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="border-b bg-muted/50">
                <tr>
                  <th className="text-left p-4 font-medium">Username</th>
                  <th className="text-left p-4 font-medium">Email</th>
                  <th className="text-left p-4 font-medium">Role</th>
                  <th className="text-left p-4 font-medium">Created</th>
                  <th className="text-right p-4 font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {users?.map((user) => (
                  <tr key={user.id} className="border-b last:border-0 hover:bg-muted/50">
                    <td className="p-4 font-medium">
                      {user.username}
                      {user.id === currentUser?.id && (
                        <span className="ml-2 text-xs text-muted-foreground">(you)</span>
                      )}
                    </td>
                    <td className="p-4 text-sm text-muted-foreground">{user.email}</td>
                    <td className="p-4">
                      {user.is_admin ? (
                        <Badge className="bg-purple-500/10 text-purple-500 border-purple-500/20">
                          <Shield className="h-3 w-3 mr-1" />
                          Admin
                        </Badge>
                      ) : (
                        <Badge variant="outline">User</Badge>
                      )}
                    </td>
                    <td className="p-4 text-sm text-muted-foreground">
                      {new Date(user.created_at).toLocaleDateString()}
                    </td>
                    <td className="p-4">
                      <div className="flex justify-end gap-2">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() =>
                            setConfirmAction({
                              type: 'toggle',
                              userId: user.id,
                              username: user.username,
                              currentIsAdmin: user.is_admin,
                            })
                          }
                          disabled={toggleAdminMutation.isPending || user.id === currentUser?.id}
                          title={
                            user.id === currentUser?.id
                              ? 'Cannot modify your own admin status'
                              : user.is_admin
                              ? 'Revoke Admin'
                              : 'Grant Admin'
                          }
                        >
                          {user.is_admin ? (
                            <ShieldOff className="h-4 w-4" />
                          ) : (
                            <Shield className="h-4 w-4" />
                          )}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            if (user.id === currentUser?.id) return;
                            setConfirmAction({
                              type: 'delete',
                              userId: user.id,
                              username: user.username,
                            });
                          }}
                          disabled={deleteUserMutation.isPending || user.id === currentUser?.id}
                          title={
                            user.id === currentUser?.id
                              ? 'Cannot delete yourself'
                              : 'Delete User'
                          }
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

      {users?.length === 0 && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">No users found</p>
        </div>
      )}

      <ConfirmDialog
        open={!!confirmAction}
        onOpenChange={(open) => !open && setConfirmAction(null)}
        onConfirm={handleConfirmAction}
        title={
          confirmAction?.type === 'toggle'
            ? confirmAction.currentIsAdmin
              ? 'Revoke Admin Access'
              : 'Grant Admin Access'
            : 'Delete User'
        }
        description={
          confirmAction?.type === 'toggle'
            ? confirmAction.currentIsAdmin
              ? `Are you sure you want to revoke admin access for ${confirmAction.username}? They will lose all admin privileges.`
              : `Are you sure you want to grant admin access to ${confirmAction.username}? They will have full system access.`
            : `Are you sure you want to delete ${confirmAction?.username}? This action cannot be undone. All their environments and data will be permanently removed.`
        }
        confirmText={confirmAction?.type === 'delete' ? 'Delete' : 'Confirm'}
        cancelText="Cancel"
        variant={confirmAction?.type === 'delete' ? 'destructive' : 'default'}
      />
    </div>
  );
};
