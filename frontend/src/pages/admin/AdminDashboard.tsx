import { Link } from 'react-router-dom';
import { useUsers, useAuditLogs, useDashboardStats } from '@/hooks/useAdmin';
import { useEnvironments } from '@/hooks/useEnvironments';
import { useJobs } from '@/hooks/useJobs';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Loader2, Users, Boxes, Activity, ListTodo, HardDrive, Package } from 'lucide-react';

const StatCard = ({ title, value, icon: Icon }: { title: string; value: number | string; icon: any }) => {
  return (
    <Card>
      <CardContent className="p-6">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-muted-foreground">{title}</p>
            <p className="text-2xl font-bold mt-2">{value}</p>
          </div>
          <Icon className="h-8 w-8 text-muted-foreground" />
        </div>
      </CardContent>
    </Card>
  );
};

export const AdminDashboard = () => {
  const { data: users, isLoading: usersLoading } = useUsers();
  const { data: environments, isLoading: envsLoading } = useEnvironments();
  const { data: jobs, isLoading: jobsLoading } = useJobs();
  const { data: auditLogs, isLoading: logsLoading } = useAuditLogs();
  const { data: dashboardStats, isLoading: statsLoading } = useDashboardStats();

  const activeJobs = jobs?.filter(job => job.status === 'running' || job.status === 'pending').length || 0;

  if (usersLoading || envsLoading || jobsLoading || logsLoading || statsLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold">Admin Dashboard</h1>
        <p className="text-muted-foreground">System overview and management</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
        <StatCard title="Total Users" value={users?.length || 0} icon={Users} />
        <StatCard title="Environments" value={environments?.length || 0} icon={Boxes} />
        <StatCard title="Active Jobs" value={activeJobs} icon={ListTodo} />
        <StatCard title="Audit Logs" value={auditLogs?.length || 0} icon={Activity} />
        <StatCard title="Disk Usage" value={dashboardStats?.total_disk_usage_formatted || 'Calculating...'} icon={HardDrive} />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Recent Activity</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {auditLogs?.slice(0, 10).map((log) => (
              <div
                key={log.id}
                className="flex items-center justify-between p-3 rounded-lg border"
              >
                <div className="flex-1">
                  <p className="text-sm font-medium">
                    {log.user?.username || 'Unknown User'}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {log.action.replace(/_/g, ' ')} - {log.resource}
                  </p>
                </div>
                <span className="text-xs text-muted-foreground">
                  {new Date(log.timestamp).toLocaleString()}
                </span>
              </div>
            ))}
          </div>
          {(!auditLogs || auditLogs.length === 0) && (
            <p className="text-sm text-muted-foreground text-center py-8">
              No audit logs yet
            </p>
          )}
        </CardContent>
      </Card>

      <div className="flex gap-4">
        <Link to="/admin/users">
          <Button>
            <Users className="h-4 w-4 mr-2" />
            Manage Users
          </Button>
        </Link>
        <Link to="/admin/registries">
          <Button variant="outline">
            <Package className="h-4 w-4 mr-2" />
            Manage Registries
          </Button>
        </Link>
        <Link to="/admin/audit-logs">
          <Button variant="outline">
            <Activity className="h-4 w-4 mr-2" />
            View All Logs
          </Button>
        </Link>
      </div>
    </div>
  );
};
