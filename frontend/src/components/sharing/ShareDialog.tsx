import { useState } from 'react';
import { useCollaborators, useShareWorkspace, useUnshareWorkspace, useGroups, useShareWorkspaceWithGroup, useUnshareWorkspaceFromGroup } from '@/hooks/useAdmin';
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
import { Loader2, Users, X } from 'lucide-react';

interface ShareDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  environmentId: string;
}

export const ShareDialog = ({ open, onOpenChange, environmentId }: ShareDialogProps) => {
  const [selectedUser, setSelectedUser] = useState('');
  const [selectedRole, setSelectedRole] = useState<'editor' | 'viewer'>('viewer');
  const [selectedGroup, setSelectedGroup] = useState('');
  const [selectedGroupRole, setSelectedGroupRole] = useState<'editor' | 'viewer'>('viewer');
  const [confirmRemove, setConfirmRemove] = useState<{ userId?: string; groupName?: string; displayName: string } | null>(null);
  const [error, setError] = useState('');

  const { data: collaborators, isLoading: collaboratorsLoading } = useCollaborators(environmentId, open);
  const { data: allUsers } = useUsers();
  const { data: allGroups } = useGroups();
  const shareMutation = useShareWorkspace(environmentId);
  const unshareMutation = useUnshareWorkspace(environmentId);
  const shareGroupMutation = useShareWorkspaceWithGroup(environmentId);
  const unshareGroupMutation = useUnshareWorkspaceFromGroup(environmentId);

  const availableUsers = allUsers?.filter(
    (user) => !collaborators?.some((c) => !c.is_group && c.user_id === user.id)
  );

  // Filter out groups that already have access
  const availableGroups = allGroups?.filter(
    (group) => !collaborators?.some((c) => c.is_group && c.group_name === group)
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

  const handleShareGroup = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedGroup) return;

    setError('');
    try {
      await shareGroupMutation.mutateAsync({
        group_name: selectedGroup,
        role: selectedGroupRole,
      });

      setSelectedGroup('');
      setSelectedGroupRole('viewer');
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to share with group. Please try again.';
      setError(errorMessage);
    }
  };

  const handleUnshare = async () => {
    if (!confirmRemove) return;

    setError('');
    try {
      if (confirmRemove.groupName) {
        await unshareGroupMutation.mutateAsync(confirmRemove.groupName);
      } else if (confirmRemove.userId) {
        await unshareMutation.mutateAsync(confirmRemove.userId);
      }
      setConfirmRemove(null);
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to remove access. Please try again.';
      setError(errorMessage);
      setConfirmRemove(null);
    }
  };

  const userCollaborators = collaborators?.filter((c) => !c.is_group) ?? [];
  const groupCollaborators = collaborators?.filter((c) => c.is_group) ?? [];

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
                {userCollaborators.map((collab) => (
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
                      {!collab.is_owner && collab.user_id && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setConfirmRemove({ userId: collab.user_id, displayName: collab.username || '' })}
                          disabled={unshareMutation.isPending}
                        >
                          <X className="h-4 w-4" />
                        </Button>
                      )}
                    </div>
                  </div>
                ))}
                {groupCollaborators.map((collab) => (
                  <div
                    key={`group-${collab.group_name}`}
                    className="flex justify-between items-center p-3 border rounded-lg"
                  >
                    <div className="flex-1 flex items-center gap-2">
                      <Users className="h-4 w-4 text-muted-foreground" />
                      <div className="font-medium">{collab.group_name}</div>
                    </div>
                    <div className="flex items-center gap-2">
                      <RoleBadge role={collab.role} />
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setConfirmRemove({ groupName: collab.group_name, displayName: collab.group_name || '' })}
                        disabled={unshareGroupMutation.isPending}
                      >
                        <X className="h-4 w-4" />
                      </Button>
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

            {availableGroups && availableGroups.length > 0 && (
              <div className="space-y-3">
                <h3 className="text-sm font-semibold">Add Group</h3>
                <form onSubmit={handleShareGroup} className="space-y-3">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Group</label>
                    <SelectRoot value={selectedGroup} onValueChange={setSelectedGroup}>
                      <SelectTrigger className="w-full">
                        <SelectValue placeholder="Select group..." />
                      </SelectTrigger>
                      <SelectContent>
                        {availableGroups.map((group) => (
                          <SelectItem key={group} value={group}>
                            {group}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </SelectRoot>
                  </div>

                  <div className="space-y-2">
                    <label className="text-sm font-medium">Access Level</label>
                    <SelectRoot
                      value={selectedGroupRole}
                      onValueChange={(value) => setSelectedGroupRole(value as 'editor' | 'viewer')}
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
                    disabled={!selectedGroup || shareGroupMutation.isPending}
                    className="w-full"
                  >
                    {shareGroupMutation.isPending ? (
                      <>
                        <Loader2 className="h-4 w-4 animate-spin mr-2" />
                        Adding...
                      </>
                    ) : (
                      'Add Group'
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
        title={confirmRemove?.groupName ? 'Remove Group Access' : 'Remove Collaborator'}
        description={
          confirmRemove?.groupName
            ? `Are you sure you want to remove the group "${confirmRemove.displayName}" from this workspace? All group members will lose access immediately.`
            : `Are you sure you want to remove ${confirmRemove?.displayName} from this workspace? They will lose access immediately.`
        }
        confirmText="Remove"
        cancelText="Cancel"
        variant="destructive"
      />
    </Dialog>
  );
};
