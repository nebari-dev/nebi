import { Users } from 'lucide-react';
import { useMemo, useState } from 'react';
import { GroupMembersDialog } from '@/components/admin/GroupMembersDialog';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useGroups } from '@/hooks/useGroups';
import type { GroupWithMemberCount } from '@/types/models';

// Groups and their memberships come from the identity provider's groups
// claim, so this page is read-only. Granting a group access to projects and
// registries happens from those resources.
export const Groups = () => {
  const { data: groups, isLoading } = useGroups();
  const [membersOf, setMembersOf] = useState<GroupWithMemberCount | null>(null);

  const rows = useMemo(() => groups ?? [], [groups]);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold">Groups</h1>
        <p className="text-muted-foreground">
          Groups are synced from your identity provider when users sign in.
          Grant them permission to projects and registries.
        </p>
      </div>

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
              <TableHead>Members</TableHead>
              <TableHead>Created</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((g) => (
              <TableRow key={g.id}>
                <TableCell className="font-medium">{g.name}</TableCell>
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
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

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
