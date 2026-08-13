import { Loader2, Search } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { useAuditLogs } from '@/hooks/useAdmin';
import { useRemoteAuditLogs, useRemoteServer } from '@/hooks/useRemote';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';

const ACTION_COLORS: Record<string, string> = {
  create_user: 'bg-green-100 text-green-800 border-green-300',
  delete_user: 'bg-red-100 text-red-800 border-red-300',
  grant_permission: 'bg-blue-100 text-blue-800 border-blue-300',
  revoke_permission: 'bg-orange-100 text-orange-800 border-orange-300',
  make_admin: 'bg-purple-100 text-purple-800 border-purple-300',
  revoke_admin: 'bg-zinc-100 text-zinc-800 border-zinc-300',
  share_workspace: 'bg-cyan-100 text-cyan-800 border-cyan-300',
  unshare_workspace: 'bg-yellow-100 text-yellow-800 border-yellow-300',
};

export const AuditLogs = () => {
  const [filters, setFilters] = useState({
    user_id: '',
    action: '',
  });

  // View mode support
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const viewMode = useViewModeStore((state) => state.viewMode);
  const { data: serverStatus } = useRemoteServer();
  const isRemoteConnected = isLocalMode && serverStatus?.status === 'connected';
  const shouldShowRemote = isRemoteConnected && viewMode === 'remote';

  const { data: logs, isLoading: logsLoading } = useAuditLogs(
    filters.user_id || filters.action ? filters : undefined,
  );

  const { data: remoteLogs, isLoading: remoteLoading } = useRemoteAuditLogs(
    shouldShowRemote,
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

  const isLoading = logsLoading || (shouldShowRemote && remoteLoading);

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
          onChange={(e) => setFilters({ ...filters, action: e.target.value })}
          className="w-64"
          aria-label="Filter audit logs by action"
        >
          <option value="">All Actions</option>
          <option value="create_user">Create User</option>
          <option value="delete_user">Delete User</option>
          <option value="grant_permission">Grant Permission</option>
          <option value="revoke_permission">Revoke Permission</option>
          <option value="make_admin">Make Admin</option>
          <option value="revoke_admin">Revoke Admin</option>
          <option value="share_workspace">Share Workspace</option>
          <option value="unshare_workspace">Unshare Workspace</option>
        </Select>
      </div>

      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="border-b bg-muted/50">
                <tr>
                  <th className="text-left p-4 font-medium">Timestamp</th>
                  <th className="text-left p-4 font-medium">User</th>
                  <th className="text-left p-4 font-medium">Action</th>
                  <th className="text-left p-4 font-medium">Resource</th>
                  <th className="text-left p-4 font-medium">Details</th>
                </tr>
              </thead>
              <tbody>
                {displayedLogs.map((log) => (
                  <tr
                    key={log.id}
                    className="border-b last:border-0 hover:bg-muted/50"
                  >
                    <td className="p-4 text-sm text-muted-foreground whitespace-nowrap">
                      {new Date(log.timestamp).toLocaleString()}
                    </td>
                    <td className="p-4 font-medium">
                      {log.user?.username || log.user_id}
                    </td>
                    <td className="p-4">
                      <Badge
                        className={
                          ACTION_COLORS[log.action] ||
                          'bg-zinc-100 text-zinc-800 border-zinc-300'
                        }
                      >
                        {log.action.replace(/_/g, ' ')}
                      </Badge>
                    </td>
                    <td className="p-4 font-mono text-sm">{log.resource}</td>
                    <td className="p-4 text-sm text-muted-foreground">
                      {log.details_json && (
                        <details className="cursor-pointer">
                          <summary className="hover:text-foreground">
                            View Details
                          </summary>
                          <pre className="mt-2 p-2 bg-muted rounded text-xs overflow-auto max-w-md">
                            {JSON.stringify(log.details_json, null, 2)}
                          </pre>
                        </details>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {displayedLogs.length === 0 && (
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
