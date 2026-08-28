import { Trash2, Users } from 'lucide-react';
import { useMemo, useState } from 'react';
import { CreateGroupDialog } from '@/components/admin/CreateGroupDialog';
import { GroupMembersDialog } from '@/components/admin/GroupMembersDialog';
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
import { useDeleteGroup, useGroups } from '@/hooks/useGroups';
import type { GroupWithMemberCount } from '@/types/models';

export const Groups = () => {
  const { data: groups, isLoading } = useGroups();
  const deleteMutation = useDeleteGroup();
  const [confirm, setConfirm] = useState<{ id: string; name: string } | null>(
    null,
  );
  const [membersOf, setMembersOf] = useState<GroupWithMemberCount | null>(null);
  const [error, setError] = useState('');

  const rows = useMemo(() => groups ?? [], [groups]);

  const handleDelete = async () => {
    if (!confirm) return;
    setError('');
    try {
      await deleteMutation.mutateAsync(confirm.id);
      setConfirm(null);
    } catch (err) {
      setError(
        (err as { response?: { data?: { error?: string } } })?.response?.data
          ?.error ?? 'Failed to delete group',
      );
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold">Groups</h1>
          <p className="text-muted-foreground">
            Manage groups and grant permission to workspaces and registries.
          </p>
        </div>
        <CreateGroupDialog />
      </div>

      {error && (
        <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded">
          {error}
        </div>
      )}

      {isLoading ? (
        <div className="rounded-md border border-border bg-card p-8 text-center text-muted-foreground">
          Loading…
        </div>
      ) : rows.length === 0 ? (
        <div className="rounded-md border border-border bg-card p-8 text-center text-muted-foreground">
          No groups yet.
        </div>
      ) : (
        <Table aria-label="Groups">
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Description</TableHead>
              <TableHead>Source</TableHead>
              <TableHead>Members</TableHead>
              <TableHead>Created</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((g) => (
              <TableRow key={g.id}>
                <TableCell className="font-medium">{g.name}</TableCell>
                <TableCell className="text-muted-foreground">
                  {g.description}
                </TableCell>
                <TableCell>
                  <Badge
                    variant="outline"
                    className={
                      g.source === 'oidc'
                        ? 'border-blue-500/40 text-blue-500'
                        : ''
                    }
                  >
                    {g.source}
                  </Badge>
                </TableCell>
                <TableCell>{g.member_count}</TableCell>
                <TableCell className="text-muted-foreground">
                  {new Date(g.created_at).toLocaleDateString()}
                </TableCell>
                <TableCell>
                  <div className="flex justify-end gap-2">
                    <Button
                      variant="ghost"
                      size="sm"
                      title="View members"
                      aria-label={`View members of ${g.name}`}
                      onClick={() => setMembersOf(g)}
                    >
                      <Users className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      title={
                        g.source === 'oidc'
                          ? 'OIDC groups cannot be deleted'
                          : 'Delete group'
                      }
                      disabled={g.source === 'oidc'}
                      aria-label={`Delete ${g.name}`}
                      onClick={() => setConfirm({ id: g.id, name: g.name })}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <ConfirmDialog
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(null)}
        onConfirm={handleDelete}
        title="Delete group"
        description={`Delete group "${confirm?.name}"? Members lose all permissions granted via this group.`}
        confirmText="Delete"
        cancelText="Cancel"
        variant="destructive"
      />

      {membersOf && (
        <GroupMembersDialog
          group={membersOf}
          open={!!membersOf}
          onOpenChange={(o) => !o && setMembersOf(null)}
        />
      )}
    </div>
  );
};
