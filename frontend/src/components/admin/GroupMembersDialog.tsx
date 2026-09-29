import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useGroupMembers } from '@/hooks/useGroups';
import type { GroupWithMemberCount } from '@/types/models';

interface Props {
  group: GroupWithMemberCount;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// Read-only: membership is synced from the identity provider's groups claim.
export const GroupMembersDialog = ({ group, open, onOpenChange }: Props) => {
  const { data: members, isLoading } = useGroupMembers(group.id);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{group.name}</DialogTitle>
        </DialogHeader>

        <div className="space-y-2">
          <h3 className="text-sm font-medium">Members</h3>
          {isLoading ? (
            <div className="text-sm text-muted-foreground">Loading…</div>
          ) : members && members.length > 0 ? (
            <ul className="divide-y border rounded">
              {members.map((m) => (
                <li key={m.user_id} className="p-2">
                  <div className="font-medium text-sm">
                    {m.user?.username ?? m.user_id}
                  </div>
                  {m.user?.email && (
                    <div className="text-xs text-muted-foreground">
                      {m.user.email}
                    </div>
                  )}
                </li>
              ))}
            </ul>
          ) : (
            <div className="text-sm text-muted-foreground">No members.</div>
          )}
          <p className="text-xs text-muted-foreground">
            Membership is managed in your identity provider and updates when
            members sign in.
          </p>
        </div>
      </DialogContent>
    </Dialog>
  );
};
