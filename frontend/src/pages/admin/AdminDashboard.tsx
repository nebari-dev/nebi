import {
  Activity,
  AlertTriangle,
  Boxes,
  HardDrive,
  Loader2,
  Package,
  Users,
  Users2,
} from 'lucide-react';
import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { RemoteUnreachableBanner } from '@/components/remote/RemoteUnreachableBanner';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Card, CardContent } from '@/components/ui/card';
import { useDashboardStats, useUsers } from '@/hooks/useAdmin';
import { useJobs } from '@/hooks/useJobs';
import { useProjects } from '@/hooks/useProjects';
import {
  useRemoteDashboardStats,
  useRemoteJobs,
  useRemoteProjects,
  useRemoteView,
} from '@/hooks/useRemote';

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
      <CardContent>
        <div className="flex items-center gap-4">
          <div className="rounded-lg bg-primary/10 p-3">
            <Icon className="h-5 w-5 text-primary" />
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
    title: 'View Users',
    description: 'See who has signed in and their roles',
    icon: Users,
    to: '/admin/users',
  },
  {
    title: 'Manage Registries',
    description: 'Configure package registries',
    icon: Package,
    to: '/admin/registries',
  },
  {
    title: 'View Groups',
    description: 'Browse identity provider groups',
    icon: Users2,
    to: '/admin/groups',
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
  const { data: projects, isLoading: projectLoading } = useProjects();
  const { data: jobs, isLoading: jobsLoading } = useJobs();
  const { data: dashboardStats, isLoading: statsLoading } = useDashboardStats();

  // View mode support
  const { viewMode, isRemoteConnected, isRemoteView } = useRemoteView();

  // Remote data
  const remoteProjectsQuery = useRemoteProjects(isRemoteView);
  const remoteJobsQuery = useRemoteJobs(isRemoteView);
  const remoteStatsQuery = useRemoteDashboardStats(isRemoteView);
  const remoteProjects = remoteProjectsQuery.data;
  const remoteJobs = remoteJobsQuery.data;
  const remoteDashboardStats = remoteStatsQuery.data;

  // Select data based on view mode
  const displayedProjects = useMemo(() => {
    if (!isRemoteConnected || viewMode === 'local') {
      return projects || [];
    }
    return remoteProjects || [];
  }, [projects, remoteProjects, isRemoteConnected, viewMode]);

  const displayedJobs = useMemo(() => {
    if (!isRemoteConnected || viewMode === 'local') {
      return jobs || [];
    }
    return remoteJobs || [];
  }, [jobs, remoteJobs, isRemoteConnected, viewMode]);

  const displayedStats = useMemo(() => {
    if (!isRemoteConnected || viewMode === 'local') {
      return dashboardStats;
    }
    return remoteDashboardStats;
  }, [dashboardStats, remoteDashboardStats, isRemoteConnected, viewMode]);

  const activeJobs = displayedJobs.filter(
    (job) => job.status === 'running' || job.status === 'pending',
  ).length;

  const failedJobs = displayedJobs.filter(
    (job) => job.status === 'failed',
  ).length;

  const remoteRequiredQueries = [
    remoteProjectsQuery,
    remoteJobsQuery,
    remoteStatsQuery,
  ];
  const remoteUnreachable =
    isRemoteView && remoteRequiredQueries.some((query) => query.isUnreachable);
  // Full-page spinner only until the remote queries first resolve or error
  // (see isFirstLoad in useRemote.ts).
  const isLoading =
    usersLoading ||
    projectLoading ||
    jobsLoading ||
    statsLoading ||
    (isRemoteView && remoteRequiredQueries.some((query) => query.isFirstLoad));

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  const alerts: string[] = [];
  if (failedJobs > 0) {
    alerts.push(
      `${failedJobs} job${failedJobs > 1 ? 's' : ''} failed recently`,
    );
  }

  return (
    <div className="space-y-6">
      {remoteUnreachable && <RemoteUnreachableBanner />}

      {/* Stat Cards */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <StatCard title="Total Users" value={users?.length || 0} icon={Users} />
        <StatCard
          title="Environments"
          value={displayedProjects.length}
          icon={Boxes}
        />
        <StatCard title="Active Jobs" value={activeJobs} icon={Activity} />
        <StatCard
          title="Disk Usage"
          value={displayedStats?.total_disk_usage_formatted || 'N/A'}
          icon={HardDrive}
        />
      </div>

      {/* Alert Banner */}
      {alerts.length > 0 && (
        <Alert variant="warning">
          <AlertTriangle className="h-5 w-5 shrink-0" />
          <AlertTitle>System Alerts</AlertTitle>
          <AlertDescription>{alerts.join(' \u00B7 ')}</AlertDescription>
        </Alert>
      )}

      {/* Quick Actions */}
      <div>
        <h3 className="text-lg font-semibold mb-3">Quick Actions</h3>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          {quickActions.map(({ title, description, icon: Icon, to }) => (
            <Link key={title} to={to}>
              <Card className="h-full transition-colors hover:border-primary/30 hover:bg-primary/5">
                <CardContent>
                  <div className="rounded-lg bg-primary/10 p-2 w-fit mb-3">
                    <Icon className="h-4 w-4 text-primary" />
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
