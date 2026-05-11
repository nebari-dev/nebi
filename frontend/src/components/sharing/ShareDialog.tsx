import { useState } from 'react';
import { useCollaborators, useShareWorkspace, useUnshareWorkspace } from '@/hooks/useAdmin';
import { useUsers } from '@/hooks/useAdmin';
import { useMyGroups } from '@/hooks/useGroups';
import { groupsApi } from '@/api/groups';
import { useMutation, useQueryClient } from '@tanstack/react-query';
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
import type { Collaborator } from '@/types/models';

type UserCollaborator = Extract<Collaborator, { kind: 'user' }>;
type GroupCollaboratorEntry = Extract<Collaborator, { kind: 'group' }>;

interface ShareDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  environmentId: string;
}

export const ShareDialog = ({ open, onOpenChange, environmentId }: ShareDialogProps) => {
  const [mode, setMode] = useState<'user' | 'group'>('user');
  const [selectedUser, setSelectedUser] = useState('');
  const [selectedGroup, setSelectedGroup] = useState('');
  const [selectedRole, setSelectedRole] = useState<'editor' | 'viewer'>('viewer');
  const [confirmRemove, setConfirmRemove] = useState<
    { kind: 'user' | 'group'; id: string; label: string } | null
  >(null);
  const [error, setError] = useState('');

  const { data: collaborators, isLoading: collaboratorsLoading } = useCollaborators(environmentId, open);
  const { data: allUsers } = useUsers();
  const { data: myGroups } = useMyGroups(open && mode === 'group');
  const qc = useQueryClient();
  const shareMutation = useShareWorkspace(environmentId);
  const unshareMutation = useUnshareWorkspace(environmentId);

  const shareGroupMutation = useMutation({
    mutationFn: (data: { group_id: string; role: 'editor' | 'viewer' }) =>
      groupsApi.shareWorkspace(environmentId, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['collaborators', environmentId] }),
  });

  const unshareGroupMutation = useMutation({
    mutationFn: (groupId: string) => groupsApi.unshareWorkspace(environmentId, groupId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['collaborators', environmentId] }),
  });

  const userCollaborators = collaborators?.filter(
    (c): c is UserCollaborator => c.kind === 'user'
  );

  const availableUsers = allUsers?.filter(
    (user) => !userCollaborators?.some((c) => c.user_id === user.id)
  );

  const groupCollaboratorIds = new Set(
    collaborators
      ?.filter((c): c is GroupCollaboratorEntry => c.kind === 'group')
      .map((c) => c.group_id) ?? [],
  );
  const availableGroups = (myGroups ?? []).filter((g) => !groupCollaboratorIds.has(g.id));

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
      await shareGroupMutation.mutateAsync({ group_id: selectedGroup, role: selectedRole });
      setSelectedGroup('');
      setSelectedRole('viewer');
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      setError(error?.response?.data?.error || 'Failed to share workspace with group.');
    }
  };

  const handleUnshare = async () => {
    if (!confirmRemove) return;

    setError('');
    try {
      if (confirmRemove.kind === 'user') {
        await unshareMutation.mutateAsync(confirmRemove.id);
      } else {
        await unshareGroupMutation.mutateAsync(confirmRemove.id);
      }
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
                {collaborators?.map((collab) =>
                  collab.kind === 'user' ? (
                    <div
                      key={`u-${collab.user_id}`}
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
                            onClick={() =>
                              setConfirmRemove({
                                kind: 'user',
                                id: collab.user_id,
                                label: collab.username,
                              })
                            }
                            disabled={unshareMutation.isPending}
                          >
                            <X className="h-4 w-4" />
                          </Button>
                        )}
                      </div>
                    </div>
                  ) : (
                    <div
                      key={`g-${collab.group_id}`}
                      className="flex justify-between items-center p-3 border rounded-lg"
                    >
                      <div className="flex-1">
                        <div className="font-medium">{collab.name}</div>
                        <div className="text-sm text-muted-foreground">
                          {collab.source === 'oidc' ? 'OIDC group' : 'Native group'}
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <RoleBadge role={collab.role} />
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() =>
                            setConfirmRemove({
                              kind: 'group',
                              id: collab.group_id,
                              label: collab.name,
                            })
                          }
                          disabled={unshareGroupMutation.isPending}
                        >
                          <X className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  ),
                )}
              </div>
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold">Add Collaborator</h3>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant={mode === 'user' ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => setMode('user')}
                >
                  User
                </Button>
                <Button
                  type="button"
                  variant={mode === 'group' ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => setMode('group')}
                >
                  Group
                </Button>
              </div>

              {mode === 'user' && availableUsers && availableUsers.length > 0 && (
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
              )}

              {mode === 'user' && (!availableUsers || availableUsers.length === 0) && (
                <p className="text-sm text-muted-foreground">
                  All users are already collaborators.
                </p>
              )}

              {mode === 'group' && availableGroups.length > 0 && (
                <form onSubmit={handleShareGroup} className="space-y-3">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Group</label>
                    <SelectRoot value={selectedGroup} onValueChange={setSelectedGroup}>
                      <SelectTrigger className="w-full">
                        <SelectValue placeholder="Select group..." />
                      </SelectTrigger>
                      <SelectContent>
                        {availableGroups.map((g) => (
                          <SelectItem key={g.id} value={g.id}>
                            {g.name} ({g.source})
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
              )}

              {mode === 'group' && availableGroups.length === 0 && (
                <p className="text-sm text-muted-foreground">
                  No groups available to share with.
                </p>
              )}
            </div>
          </div>
        )}
      </DialogContent>

      <ConfirmDialog
        open={!!confirmRemove}
        onOpenChange={(open) => !open && setConfirmRemove(null)}
        onConfirm={handleUnshare}
        title="Remove Collaborator"
        description={`Are you sure you want to remove ${confirmRemove?.label} from this workspace? They will lose access immediately.`}
        confirmText="Remove"
        cancelText="Cancel"
        variant="destructive"
      />
    </Dialog>
  );
};
