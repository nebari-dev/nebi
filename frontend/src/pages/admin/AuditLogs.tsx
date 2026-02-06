import { useState } from 'react';
import { useAuditLogs } from '@/hooks/useAdmin';
import { Card, CardContent } from '@/components/ui/card';
import { Select } from '@/components/ui/select';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Loader2, Search } from 'lucide-react';

const ACTION_COLORS: Record<string, string> = {
  create_user: 'bg-green-500/10 text-green-500 border-green-500/20',
  delete_user: 'bg-red-500/10 text-red-500 border-red-500/20',
  grant_permission: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  revoke_permission: 'bg-orange-500/10 text-orange-500 border-orange-500/20',
  make_admin: 'bg-purple-500/10 text-purple-500 border-purple-500/20',
  revoke_admin: 'bg-gray-500/10 text-gray-500 border-gray-500/20',
  share_workspace: 'bg-cyan-500/10 text-cyan-500 border-cyan-500/20',
  unshare_workspace: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
  create_workspace: 'bg-emerald-500/10 text-emerald-500 border-emerald-500/20',
  delete_workspace: 'bg-red-500/10 text-red-500 border-red-500/20',
  install_package: 'bg-cyan-500/10 text-cyan-500 border-cyan-500/20',
  remove_package: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
  publish_workspace: 'bg-teal-500/10 text-teal-500 border-teal-500/20',
};

export const AuditLogs = () => {
  const [filters, setFilters] = useState({
    user_id: '',
    action: '',
  });

  const { data: logs, isLoading } = useAuditLogs(
    filters.user_id || filters.action ? filters : undefined
  );

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
        <p className="text-muted-foreground">View all system activity and changes</p>
      </div>

      <div className="flex gap-4">
        <div className="flex-1">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Filter by user ID..."
              value={filters.user_id}
              onChange={(e) => setFilters({ ...filters, user_id: e.target.value })}
              className="pl-9"
            />
          </div>
        </div>
        <Select
          value={filters.action}
          onChange={(e) => setFilters({ ...filters, action: e.target.value })}
          className="w-64"
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
          <option value="create_workspace">Create Workspace</option>
          <option value="delete_workspace">Delete Workspace</option>
          <option value="install_package">Install Package</option>
          <option value="remove_package">Remove Package</option>
          <option value="publish_workspace">Publish Workspace</option>
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
                {logs?.map((log) => (
                  <tr key={log.id} className="border-b last:border-0 hover:bg-muted/50">
                    <td className="p-4 text-sm text-muted-foreground whitespace-nowrap">
                      {new Date(log.timestamp).toLocaleString()}
                    </td>
                    <td className="p-4 font-medium">
                      {log.user?.username || log.user_id}
                    </td>
                    <td className="p-4">
                      <Badge className={ACTION_COLORS[log.action] || 'bg-gray-500/10 text-gray-500 border-gray-500/20'}>
                        {log.action.replace(/_/g, ' ')}
                      </Badge>
                    </td>
                    <td className="p-4 font-mono text-sm">{log.resource}</td>
                    <td className="p-4 text-sm text-muted-foreground">
                      {log.details_json && (
                        <details className="cursor-pointer">
                          <summary className="hover:text-foreground">View Details</summary>
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

      {logs?.length === 0 && (
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
