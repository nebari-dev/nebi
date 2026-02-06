import { useState } from 'react';
import { useCollaborators, useShareWorkspace, useUnshareWorkspace } from '@/hooks/useAdmin';
import { useUsers } from '@/hooks/useAdmin';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import {
  SelectRoot,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select-v2';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { RoleBadge } from './RoleBadge';
import { Loader2, X } from 'lucide-react';

interface ShareDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  environmentId: string;
}

export const ShareDialog = ({ open, onOpenChange, environmentId }: ShareDialogProps) => {
  const [selectedUser, setSelectedUser] = useState('');
  const [selectedRole, setSelectedRole] = useState<'editor' | 'viewer'>('viewer');
  const [confirmRemove, setConfirmRemove] = useState<{ userId: string; username: string } | null>(null);
  const [error, setError] = useState('');

  const { data: collaborators, isLoading: collaboratorsLoading } = useCollaborators(environmentId, open);
  const { data: allUsers } = useUsers();
  const shareMutation = useShareWorkspace(environmentId);
  const unshareMutation = useUnshareWorkspace(environmentId);

  const availableUsers = allUsers?.filter(
    (user) => !collaborators?.some((c) => c.user_id === user.id)
  );

  const handleShare = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedUser) return;

    setError('');
    try {
      await shareMutation.mutateAsync({
        user_id: selectedUser,
        role: selectedRole,
      });

      setSelectedUser('');
      setSelectedRole('viewer');
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to share workspace. Please try again.';
      setError(errorMessage);
    }
  };

  const handleUnshare = async () => {
    if (!confirmRemove) return;

    setError('');
    try {
      await unshareMutation.mutateAsync(confirmRemove.userId);
      setConfirmRemove(null);
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to remove access. Please try again.';
      setError(errorMessage);
      setConfirmRemove(null);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Share Workspace</DialogTitle>
          <DialogDescription>
            Manage who has access to this workspace
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-3 py-2 rounded text-sm mt-4">
            {error}
          </div>
        )}

        {collaboratorsLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <div className="space-y-6 mt-4">
            <div className="space-y-3">
              <h3 className="text-sm font-semibold">Current Access</h3>
              <div className="space-y-2 max-h-64 overflow-y-auto">
                {collaborators?.map((collab) => (
                  <div
                    key={collab.user_id}
                    className="flex justify-between items-center p-3 border rounded-lg"
                  >
                    <div className="flex-1">
                      <div className="font-medium">{collab.username}</div>
                      <div className="text-sm text-muted-foreground">{collab.email}</div>
                    </div>
                    <div className="flex items-center gap-2">
                      <RoleBadge role={collab.role} />
                      {!collab.is_owner && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setConfirmRemove({ userId: collab.user_id, username: collab.username })}
                          disabled={unshareMutation.isPending}
                        >
                          <X className="h-4 w-4" />
                        </Button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>

            {availableUsers && availableUsers.length > 0 && (
              <div className="space-y-3">
                <h3 className="text-sm font-semibold">Add Collaborator</h3>
                <form onSubmit={handleShare} className="space-y-3">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">User</label>
                    <SelectRoot value={selectedUser} onValueChange={setSelectedUser}>
                      <SelectTrigger className="w-full">
                        <SelectValue placeholder="Select user..." />
                      </SelectTrigger>
                      <SelectContent>
                        {availableUsers.map((user) => (
                          <SelectItem key={user.id} value={user.id}>
                            {user.username} ({user.email})
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </SelectRoot>
                  </div>

                  <div className="space-y-2">
                    <label className="text-sm font-medium">Access Level</label>
                    <SelectRoot
                      value={selectedRole}
                      onValueChange={(value) => setSelectedRole(value as 'editor' | 'viewer')}
                    >
                      <SelectTrigger className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="viewer">Viewer (Read-only)</SelectItem>
                        <SelectItem value="editor">Editor (Can modify)</SelectItem>
                      </SelectContent>
                    </SelectRoot>
                  </div>

                  <Button
                    type="submit"
                    disabled={!selectedUser || shareMutation.isPending}
                    className="w-full"
                  >
                    {shareMutation.isPending ? (
                      <>
                        <Loader2 className="h-4 w-4 animate-spin mr-2" />
                        Adding...
                      </>
                    ) : (
                      'Add Collaborator'
                    )}
                  </Button>
                </form>
              </div>
            )}
          </div>
        )}
      </DialogContent>

      <ConfirmDialog
        open={!!confirmRemove}
        onOpenChange={(open) => !open && setConfirmRemove(null)}
        onConfirm={handleUnshare}
        title="Remove Collaborator"
        description={`Are you sure you want to remove ${confirmRemove?.username} from this workspace? They will lose access immediately.`}
        confirmText="Remove"
        cancelText="Cancel"
        variant="destructive"
      />
    </Dialog>
  );
};
