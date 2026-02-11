import { Link } from 'react-router-dom';
import { useUsers, useDashboardStats } from '@/hooks/useAdmin';
import { useWorkspaces } from '@/hooks/useWorkspaces';
import { useJobs } from '@/hooks/useJobs';
import { Card, CardContent } from '@/components/ui/card';
import {
  Loader2,
  Users,
  Boxes,
  Activity,
  HardDrive,
  AlertTriangle,
  UserPlus,
  Package,
} from 'lucide-react';

const StatCard = ({
  title,
  value,
  icon: Icon,
}: {
  title: string;
  value: number | string;
  icon: React.ElementType;
}) => {
  return (
    <Card>
      <CardContent className="p-6">
        <div className="flex items-center gap-4">
          <div className="rounded-lg bg-[#F5EFFE] p-3">
            <Icon className="h-5 w-5 text-[#9B3DCC]" />
          </div>
          <div>
            <p className="text-sm text-muted-foreground">{title}</p>
            <p className="text-2xl font-bold">{value}</p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
};

const quickActions = [
  {
    title: 'Manage Users',
    description: 'Add users and manage permissions',
    icon: UserPlus,
    to: '/admin/users',
  },
  {
    title: 'Manage Registries',
    description: 'Configure package registries',
    icon: Package,
    to: '/admin/registries',
  },
  {
    title: 'View Audit Logs',
    description: 'Review system activity and events',
    icon: Activity,
    to: '/admin/audit-logs',
  },
];

export const AdminDashboard = () => {
  const { data: users, isLoading: usersLoading } = useUsers();
  const { data: workspaces, isLoading: wsLoading } = useWorkspaces();
  const { data: jobs, isLoading: jobsLoading } = useJobs();
  const { data: dashboardStats, isLoading: statsLoading } = useDashboardStats();

  const activeJobs =
    jobs?.filter(
      (job) => job.status === 'running' || job.status === 'pending',
    ).length || 0;

  const failedJobs =
    jobs?.filter((job) => job.status === 'failed').length || 0;

  if (usersLoading || wsLoading || jobsLoading || statsLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  const alerts: string[] = [];
  if (failedJobs > 0) {
    alerts.push(`${failedJobs} job${failedJobs > 1 ? 's' : ''} failed recently`);
  }

  return (
    <div className="space-y-6">
      {/* Stat Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard title="Total Users" value={users?.length || 0} icon={Users} />
        <StatCard
          title="Environments"
          value={workspaces?.length || 0}
          icon={Boxes}
        />
        <StatCard title="Active Jobs" value={activeJobs} icon={Activity} />
        <StatCard
          title="Disk Usage"
          value={dashboardStats?.total_disk_usage_formatted || 'N/A'}
          icon={HardDrive}
        />
      </div>

      {/* Alert Banner */}
      {alerts.length > 0 && (
        <Card className="border-amber-300 bg-amber-50">
          <CardContent className="flex items-center gap-3 p-4">
            <AlertTriangle className="h-5 w-5 shrink-0 text-amber-600" />
            <div>
              <p className="text-sm font-medium text-amber-800">
                System Alerts
              </p>
              <p className="text-sm text-amber-700">
                {alerts.join(' \u00B7 ')}
              </p>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Quick Actions */}
      <div>
        <h3 className="text-lg font-semibold mb-3">Quick Actions</h3>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          {quickActions.map(({ title, description, icon: Icon, to }) => (
            <Link key={title} to={to}>
              <Card className="h-full transition-colors hover:border-[#9B3DCC]/30 hover:bg-[#F5EFFE]/50">
                <CardContent className="p-5">
                  <div className="rounded-lg bg-[#F5EFFE] p-2 w-fit mb-3">
                    <Icon className="h-4 w-4 text-[#9B3DCC]" />
                  </div>
                  <p className="text-sm font-medium">{title}</p>
                  <p className="text-xs text-muted-foreground mt-1">
                    {description}
                  </p>
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      </div>
    </div>
  );
};
