import { Loader2, Shield } from 'lucide-react';
import { useMemo } from 'react';
import { RemoteUnreachableBanner } from '@/components/remote/RemoteUnreachableBanner';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useUserGroups, useUsers } from '@/hooks/useAdmin';
import { useRemoteUsers, useRemoteView } from '@/hooks/useRemote';
import { useAuthStore } from '@/store/authStore';

interface UserGroupsCellProps {
  userId: string;
}

const UserGroupsCell = ({ userId }: UserGroupsCellProps) => {
  const { data: groups, isLoading } = useUserGroups(userId);
  if (isLoading)
    return <span className="text-xs text-muted-foreground">…</span>;
  if (!groups || groups.length === 0)
    return <span className="text-xs text-muted-foreground">-</span>;
  return (
    <div className="flex flex-wrap gap-1">
      {groups.map((g) => (
        <Badge key={g.id} variant="outline">
          {g.name}
        </Badge>
      ))}
    </div>
  );
};

// Users are provisioned by the identity provider on first sign-in, and admin
// status comes from IdP groups in the server config, so this page is a
// read-only directory.
export const UserManagement = () => {
  const { data: users, isLoading: usersLoading } = useUsers();
  const currentUser = useAuthStore((state) => state.user);

  // View mode support
  const { viewMode, isRemoteConnected, isRemoteView } = useRemoteView();
  const {
    data: remoteUsers,
    isFirstLoad: remoteFirstLoad,
    isUnreachable: remoteIsUnreachable,
  } = useRemoteUsers(isRemoteView);

  // Show users based on view mode
  const displayedUsers = useMemo(() => {
    if (!isRemoteConnected) {
      return users || [];
    }
    if (viewMode === 'local') {
      return users || [];
    } else {
      return remoteUsers || [];
    }
  }, [users, remoteUsers, isRemoteConnected, viewMode]);

  const remoteUnreachable = isRemoteView && remoteIsUnreachable;
  // Full-page spinner only until the remote list first resolves or errors
  // (see isFirstLoad in useRemote.ts).
  const isLoading = usersLoading || (isRemoteView && remoteFirstLoad);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold">User Management</h1>
        <p className="text-muted-foreground">
          Users are created on first sign-in. Admin access and group membership
          are managed in your identity provider.
        </p>
      </div>

      {remoteUnreachable && <RemoteUnreachableBanner />}

      <Table aria-label="Users">
        <TableHeader>
          <TableRow
            className={displayedUsers.length > 0 ? undefined : 'border-0'}
          >
            <TableHead>Username</TableHead>
            <TableHead>Email</TableHead>
            <TableHead>Role</TableHead>
            <TableHead>Groups</TableHead>
            <TableHead>Created</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {displayedUsers.map((user) => (
            <TableRow key={user.id}>
              <TableCell className="font-medium">
                {user.username}
                {user.id === currentUser?.id && (
                  <span className="ml-2 text-xs text-muted-foreground">
                    (you)
                  </span>
                )}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {user.email}
              </TableCell>
              <TableCell>
                {user.is_admin ? (
                  <Badge className="bg-purple-100 text-purple-800 border-purple-300">
                    <Shield className="h-3 w-3 mr-1" />
                    Admin
                  </Badge>
                ) : (
                  <Badge variant="outline">User</Badge>
                )}
              </TableCell>
              <TableCell>
                <UserGroupsCell userId={user.id} />
              </TableCell>
              <TableCell className="text-muted-foreground">
                {new Date(user.created_at).toLocaleDateString()}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      {displayedUsers.length === 0 && !remoteUnreachable && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">No users found</p>
        </div>
      )}
    </div>
  );
};
