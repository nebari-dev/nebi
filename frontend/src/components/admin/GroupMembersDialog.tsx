import { Trash2, UserPlus } from 'lucide-react';
import { useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  SelectContent,
  SelectItem,
  SelectRoot,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select-v2';
import { useUsers } from '@/hooks/useAdmin';
import {
  useAddGroupMember,
  useGroupMembers,
  useRemoveGroupMember,
} from '@/hooks/useGroups';
import type { GroupWithMemberCount } from '@/types/models';

interface Props {
  group: GroupWithMemberCount;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const GroupMembersDialog = ({ group, open, onOpenChange }: Props) => {
  const { data: members, isLoading } = useGroupMembers(group.id);
  const { data: users } = useUsers();
  const addMutation = useAddGroupMember();
  const removeMutation = useRemoveGroupMember();
  const [selectedUser, setSelectedUser] = useState('');
  const [error, setError] = useState('');

  const isOIDC = group.source === 'oidc';
  const availableUsers = (users ?? []).filter(
    (u) => !members?.some((m) => m.user_id === u.id),
  );

  const handleAdd = async () => {
    if (!selectedUser) return;
    setError('');
    try {
      await addMutation.mutateAsync({ id: group.id, userId: selectedUser });
      setSelectedUser('');
    } catch (err) {
      setError(
        (err as { response?: { data?: { error?: string } } })?.response?.data
          ?.error ?? 'Failed to add member',
      );
    }
  };

  const handleRemove = async (userId: string) => {
    setError('');
    try {
      await removeMutation.mutateAsync({ id: group.id, userId });
    } catch (err) {
      setError(
        (err as { response?: { data?: { error?: string } } })?.response?.data
          ?.error ?? 'Failed to remove member',
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>
            <span className="flex items-center gap-2">
              {group.name}
              {isOIDC && (
                <Badge
                  variant="outline"
                  className="border-blue-500/40 text-blue-500"
                >
                  OIDC-synced
                </Badge>
              )}
            </span>
          </DialogTitle>
        </DialogHeader>

        {error && (
          <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-3 py-2 rounded text-sm">
            {error}
          </div>
        )}

        <div className="space-y-4">
          <div>
            <h3 className="text-sm font-medium mb-2">Members</h3>
            {isLoading ? (
              <div className="text-sm text-muted-foreground">Loading…</div>
            ) : members && members.length > 0 ? (
              <ul className="divide-y border rounded">
                {members.map((m) => (
                  <li
                    key={m.user_id}
                    className="flex items-center justify-between p-2"
                  >
                    <div>
                      <div className="font-medium text-sm">
                        {m.user?.username ?? m.user_id}
                      </div>
                      {m.user?.email && (
                        <div className="text-xs text-muted-foreground">
                          {m.user.email}
                        </div>
                      )}
                    </div>
                    {!isOIDC && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleRemove(m.user_id)}
                        disabled={removeMutation.isPending}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    )}
                  </li>
                ))}
              </ul>
            ) : (
              <div className="text-sm text-muted-foreground">No members.</div>
            )}
          </div>

          {!isOIDC && availableUsers.length > 0 && (
            <div className="space-y-2">
              <h3 className="text-sm font-medium">Add member</h3>
              <div className="flex gap-2">
                <SelectRoot
                  value={selectedUser}
                  onValueChange={setSelectedUser}
                >
                  <SelectTrigger className="flex-1">
                    <SelectValue placeholder="Select a user" />
                  </SelectTrigger>
                  <SelectContent>
                    {availableUsers.map((u) => (
                      <SelectItem key={u.id} value={u.id}>
                        {u.username} — {u.email}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </SelectRoot>
                <Button
                  onClick={handleAdd}
                  disabled={!selectedUser || addMutation.isPending}
                >
                  <UserPlus className="h-4 w-4 mr-1" /> Add
                </Button>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
};
