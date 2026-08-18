import {
  Activity,
  AlertTriangle,
  Boxes,
  HardDrive,
  Loader2,
  Package,
  ShieldAlert,
  UserPlus,
  Users,
} from 'lucide-react';
import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { RemoteUnreachableBanner } from '@/components/remote/RemoteUnreachableBanner';
import { Card, CardContent } from '@/components/ui/card';
import {
  useDashboardStats,
  useFederatedIdentityReviews,
  useUsers,
} from '@/hooks/useAdmin';
import { useJobs } from '@/hooks/useJobs';
import {
  useRemoteDashboardStats,
  useRemoteFederatedIdentityReviews,
  useRemoteJobs,
  useRemoteView,
  useRemoteWorkspaces,
} from '@/hooks/useRemote';
import { useWorkspaces } from '@/hooks/useWorkspaces';
import { isPendingFederatedIdentityReview } from '@/types';

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
    title: 'Review Identities',
    description: 'Review blocked federated identity links',
    icon: ShieldAlert,
    to: '/admin/identity-reviews',
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
  const { data: identityReviews, isLoading: reviewsLoading } =
    useFederatedIdentityReviews();

  // View mode support
  const { viewMode, isRemoteConnected, isRemoteView } = useRemoteView();

  // Remote data
  const remoteWorkspacesQuery = useRemoteWorkspaces(isRemoteView);
  const remoteJobsQuery = useRemoteJobs(isRemoteView);
  const remoteStatsQuery = useRemoteDashboardStats(isRemoteView);
  const remoteReviewsQuery = useRemoteFederatedIdentityReviews(isRemoteView);
  const remoteWorkspaces = remoteWorkspacesQuery.data;
  const remoteJobs = remoteJobsQuery.data;
  const remoteDashboardStats = remoteStatsQuery.data;
  const remoteIdentityReviews = remoteReviewsQuery.data;

  // Select data based on view mode
  const displayedWorkspaces = useMemo(() => {
    if (!isRemoteConnected || viewMode === 'local') {
      return workspaces || [];
    }
    return remoteWorkspaces || [];
  }, [workspaces, remoteWorkspaces, isRemoteConnected, viewMode]);

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

  const displayedIdentityReviews = useMemo(() => {
    if (!isRemoteConnected || viewMode === 'local') {
      return identityReviews || [];
    }
    return remoteIdentityReviews || [];
  }, [identityReviews, remoteIdentityReviews, isRemoteConnected, viewMode]);

  const activeJobs = displayedJobs.filter(
    (job) => job.status === 'running' || job.status === 'pending',
  ).length;

  const failedJobs = displayedJobs.filter(
    (job) => job.status === 'failed',
  ).length;
  const pendingIdentityReviews = displayedIdentityReviews.filter(
    isPendingFederatedIdentityReview,
  ).length;

  const remoteRequiredQueries = [
    remoteWorkspacesQuery,
    remoteJobsQuery,
    remoteStatsQuery,
  ];
  const remoteUnreachable =
    isRemoteView && remoteRequiredQueries.some((query) => query.isError);
  // Full-page spinner only until the remote queries first resolve or error.
  // A refetch after an error resets a query to pending, so gating on isError
  // alone would flash the spinner on every retry (issue #217).
  const isLoading =
    usersLoading ||
    wsLoading ||
    jobsLoading ||
    statsLoading ||
    reviewsLoading ||
    (isRemoteView &&
      remoteRequiredQueries.some(
        (query) => query.isLoading && query.errorUpdateCount === 0,
      ));

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
  if (pendingIdentityReviews > 0) {
    alerts.push(
      `${pendingIdentityReviews} identity review${pendingIdentityReviews > 1 ? 's' : ''} pending`,
    );
  }

  return (
    <div className="space-y-6">
      {remoteUnreachable && <RemoteUnreachableBanner />}

      {/* Stat Cards */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-5">
        <StatCard title="Total Users" value={users?.length || 0} icon={Users} />
        <StatCard
          title="Environments"
          value={displayedWorkspaces.length}
          icon={Boxes}
        />
        <StatCard title="Active Jobs" value={activeJobs} icon={Activity} />
        <StatCard
          title="Identity Reviews"
          value={pendingIdentityReviews}
          icon={ShieldAlert}
        />
        <StatCard
          title="Disk Usage"
          value={displayedStats?.total_disk_usage_formatted || 'N/A'}
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
