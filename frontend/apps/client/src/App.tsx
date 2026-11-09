// Client app routes (issue #649), matching the prototype:
//   /                         Projects list            (U1)
//   /projects/new             Create project           (U1)
//   /projects/:id             Project detail           (U2)
//   /projects/:id/new-version Create new version       (U3)
//   /projects/:id/compare     Compare versions         (U4)
//   /server                   Server                   (U5)
//   /registries               Registries               (U7)
//   /jobs, /jobs/:id          Jobs, job detail         (U8)
//   /login, /admin/*          kept from before
// Frozen: units edit only their page files.
import { Toaster } from '@nebi/ui/components/ui/toast';
import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { useTheme } from '@nebi/ui/hooks/theme-provider';
import { QueryClientProvider, useQuery } from '@tanstack/react-query';
import { Loader2 } from 'lucide-react';
import type { ReactElement } from 'react';
import { useEffect } from 'react';
import {
  BrowserRouter,
  Navigate,
  Outlet,
  Route,
  Routes,
} from 'react-router-dom';
import { adminApi } from './api/admin';
import { AdminLayout } from './components/layout/AdminLayout';
import { Layout } from './components/layout/Layout';
import { getBasePath } from './lib/basePath';
import { queryClient } from './lib/queryClient';
import { AdminDashboard } from './pages/admin/AdminDashboard';
import { AuditLogs } from './pages/admin/AuditLogs';
import { FederatedIdentityReviews } from './pages/admin/FederatedIdentityReviews';
import { Groups } from './pages/admin/Groups';
import { RegistryManagement } from './pages/admin/RegistryManagement';
import { UserManagement } from './pages/admin/UserManagement';
import { JobDetailPage } from './pages/jobs/JobDetailPage';
import { JobsPage } from './pages/jobs/JobsPage';
import { Login } from './pages/Login';
import { ComparePage } from './pages/projects/ComparePage';
import { CreateProjectPage } from './pages/projects/CreateProjectPage';
import { NewVersionPage } from './pages/projects/NewVersionPage';
import { ProjectDetailPage } from './pages/projects/ProjectDetailPage';
import { ProjectsListPage } from './pages/projects/ProjectsListPage';
import { RegistriesPage } from './pages/registries/RegistriesPage';
import { ServerPage } from './pages/server/ServerPage';
import { useAuthStore } from './store/authStore';
import { useModeStore } from './store/modeStore';

// Load mode before rendering any routes
const ModeLoader = ({ children }: { children: ReactElement }) => {
  const { loading, fetchMode } = useModeStore();

  useEffect(() => {
    fetchMode();
  }, [fetchMode]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }
  return children;
};

const PrivateRoute = ({ children }: { children: ReactElement }) => {
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated());
  const isLocalMode = useModeStore((state) => state.isLocalMode());

  // In local mode, auth is bypassed
  if (isLocalMode) return children;

  return isAuthenticated ? children : <Navigate to="/login" />;
};

const AdminRoute = () => {
  const { data: isAdmin, isLoading } = useQuery({
    queryKey: ['user', 'is_admin'],
    queryFn: async () => {
      try {
        await adminApi.getUsers();
        return true;
      } catch {
        return false;
      }
    },
    retry: false,
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!isAdmin) {
    return <Navigate to="/" replace />;
  }

  return <Outlet />;
};

/** All client routes; exported for tests (render inside a MemoryRouter). */
export const AppRoutes = () => {
  const { isDarkMode } = useTheme();
  return (
    <Routes>
      <Route path="/login" element={<Login isDarkMode={isDarkMode} />} />
      <Route
        path="/"
        element={
          <PrivateRoute>
            <Layout />
          </PrivateRoute>
        }
      >
        <Route index element={<ProjectsListPage />} />
        <Route path="projects" element={<Navigate to="/" replace />} />
        <Route path="projects/new" element={<CreateProjectPage />} />
        <Route path="projects/:id" element={<ProjectDetailPage />} />
        <Route path="projects/:id/new-version" element={<NewVersionPage />} />
        <Route path="projects/:id/compare" element={<ComparePage />} />
        <Route path="server" element={<ServerPage />} />
        <Route path="registries" element={<RegistriesPage />} />
        <Route path="jobs" element={<JobsPage />} />
        <Route path="jobs/:id" element={<JobDetailPage />} />

        <Route element={<AdminRoute />}>
          <Route element={<AdminLayout />}>
            <Route path="admin" element={<AdminDashboard />} />
            <Route path="admin/users" element={<UserManagement />} />
            <Route path="admin/groups" element={<Groups />} />
            <Route
              path="admin/identity-reviews"
              element={<FederatedIdentityReviews />}
            />
            <Route path="admin/audit-logs" element={<AuditLogs />} />
            <Route path="admin/registries" element={<RegistryManagement />} />
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
};

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <Toaster>
          <BrowserRouter basename={getBasePath()}>
            <ModeLoader>
              <AppRoutes />
            </ModeLoader>
          </BrowserRouter>
        </Toaster>
      </TooltipProvider>
    </QueryClientProvider>
  );
}

export default App;
