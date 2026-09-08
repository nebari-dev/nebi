import { Loader2, Search } from 'lucide-react';
import { useMemo, useState } from 'react';
import { RemoteUnreachableBanner } from '@/components/remote/RemoteUnreachableBanner';
import { Badge } from '@/components/ui/badge';
import { CodeBlock, CodeBlockBody } from '@/components/ui/code-block';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useAuditLogs } from '@/hooks/useAdmin';
import { useRemoteAuditLogs, useRemoteView } from '@/hooks/useRemote';

const ACTION_COLORS: Record<string, string> = {
  create_user: 'bg-green-100 text-green-800 border-green-300',
  delete_user: 'bg-red-100 text-red-800 border-red-300',
  grant_permission: 'bg-blue-100 text-blue-800 border-blue-300',
  revoke_permission: 'bg-orange-100 text-orange-800 border-orange-300',
  make_admin: 'bg-purple-100 text-purple-800 border-purple-300',
  revoke_admin: 'bg-zinc-100 text-zinc-800 border-zinc-300',
  approve_federated_identity:
    'bg-emerald-100 text-emerald-800 border-emerald-300',
  reject_federated_identity: 'bg-red-100 text-red-800 border-red-300',
  discard_federated_identity: 'bg-zinc-100 text-zinc-800 border-zinc-300',
  share_workspace: 'bg-cyan-100 text-cyan-800 border-cyan-300',
  unshare_workspace: 'bg-yellow-100 text-yellow-800 border-yellow-300',
};

const ACTION_FILTER_OPTIONS = [
  { value: '', label: 'All Actions' },
  { value: 'create_user', label: 'Create User' },
  { value: 'delete_user', label: 'Delete User' },
  { value: 'grant_permission', label: 'Grant Permission' },
  { value: 'revoke_permission', label: 'Revoke Permission' },
  { value: 'make_admin', label: 'Make Admin' },
  { value: 'revoke_admin', label: 'Revoke Admin' },
  { value: 'approve_federated_identity', label: 'Approve Federated Identity' },
  { value: 'reject_federated_identity', label: 'Reject Federated Identity' },
  { value: 'discard_federated_identity', label: 'Discard Federated Identity' },
  { value: 'share_workspace', label: 'Share Workspace' },
  { value: 'unshare_workspace', label: 'Unshare Workspace' },
];

const ACTION_FILTER_LABELS = Object.fromEntries(
  ACTION_FILTER_OPTIONS.map((option) => [option.value, option.label]),
);

export const AuditLogs = () => {
  const [filters, setFilters] = useState({
    user_id: '',
    action: '',
  });

  // View mode support
  const { viewMode, isRemoteConnected, isRemoteView } = useRemoteView();

  const { data: logs, isLoading: logsLoading } = useAuditLogs(
    filters.user_id || filters.action ? filters : undefined,
  );

  const {
    data: remoteLogs,
    isFirstLoad: remoteFirstLoad,
    isUnreachable: remoteIsUnreachable,
  } = useRemoteAuditLogs(
    isRemoteView,
    filters.user_id || filters.action ? filters : undefined,
  );

  // Show logs based on view mode
  const displayedLogs = useMemo(() => {
    if (!isRemoteConnected) {
      return logs || [];
    }
    if (viewMode === 'local') {
      return logs || [];
    } else {
      return remoteLogs || [];
    }
  }, [logs, remoteLogs, isRemoteConnected, viewMode]);

  const remoteUnreachable = isRemoteView && remoteIsUnreachable;
  // Full-page spinner only until the remote list first resolves or errors
  // (see isFirstLoad in useRemote.ts).
  const isLoading = logsLoading || (isRemoteView && remoteFirstLoad);

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
        <h1 className="text-3xl font-bold">Audit Logs</h1>
        <p className="text-muted-foreground">
          View all system activity and changes
        </p>
      </div>

      {remoteUnreachable && <RemoteUnreachableBanner />}

      <div className="flex gap-4">
        <div className="flex-1">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Filter by user ID..."
              value={filters.user_id}
              onChange={(e) =>
                setFilters({ ...filters, user_id: e.target.value })
              }
              className="pl-9"
            />
          </div>
        </div>
        <Select
          value={filters.action}
          onValueChange={(action: string | null) =>
            setFilters({ ...filters, action: action ?? '' })
          }
        >
          <SelectTrigger
            className="w-64"
            aria-label="Filter audit logs by action"
          >
            <SelectValue>
              {(value: string | null) =>
                ACTION_FILTER_LABELS[value ?? ''] ?? 'All Actions'
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {ACTION_FILTER_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <Table aria-label="Audit logs">
        <TableHeader>
          <TableRow
            className={displayedLogs.length > 0 ? undefined : 'border-0'}
          >
            <TableHead>Timestamp</TableHead>
            <TableHead>User</TableHead>
            <TableHead>Action</TableHead>
            <TableHead>Resource</TableHead>
            <TableHead>Details</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {displayedLogs.map((log) => (
            <TableRow key={log.id}>
              <TableCell className="text-muted-foreground whitespace-nowrap">
                {new Date(log.timestamp).toLocaleString()}
              </TableCell>
              <TableCell className="font-medium">
                {log.user?.username || log.user_id}
              </TableCell>
              <TableCell>
                <Badge
                  className={
                    ACTION_COLORS[log.action] ||
                    'bg-zinc-100 text-zinc-800 border-zinc-300'
                  }
                >
                  {log.action.replace(/_/g, ' ')}
                </Badge>
              </TableCell>
              <TableCell className="font-mono">{log.resource}</TableCell>
              <TableCell className="text-muted-foreground">
                {log.details_json && (
                  <details className="cursor-pointer">
                    <summary className="hover:text-foreground">
                      View Details
                    </summary>
                    <CodeBlock
                      code={JSON.stringify(log.details_json, null, 2)}
                      className="mt-2 w-full max-w-md text-xs"
                    >
                      <CodeBlockBody
                        maxLines={12}
                        aria-label="Audit log details"
                      />
                    </CodeBlock>
                  </details>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      {displayedLogs.length === 0 && !remoteUnreachable && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">
            {filters.user_id || filters.action
              ? 'No logs match your filters'
              : 'No audit logs yet'}
          </p>
        </div>
      )}
    </div>
  );
};
