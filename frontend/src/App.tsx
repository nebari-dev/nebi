import { useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate, Outlet } from 'react-router-dom';
import { QueryClientProvider, useQuery } from '@tanstack/react-query';
import { queryClient } from './lib/queryClient';
import { useAuthStore } from './store/authStore';
import { useModeStore } from './store/modeStore';
import { Login } from './pages/Login';
import { Workspaces } from './pages/Workspaces';
import { WorkspaceDetail } from './pages/WorkspaceDetail';
import { Jobs } from './pages/Jobs';
import { Settings } from './pages/Settings';
import { RemoteEnvironmentDetail } from './pages/RemoteEnvironmentDetail';
import { AdminDashboard } from './pages/admin/AdminDashboard';
import { UserManagement } from './pages/admin/UserManagement';
import { AuditLogs } from './pages/admin/AuditLogs';
import { RegistryManagement } from './pages/admin/RegistryManagement';
import { Layout } from './components/layout/Layout';
import { adminApi } from './api/admin';
import { Loader2 } from 'lucide-react';

const PrivateRoute = ({ children }: { children: React.ReactNode }) => {
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated());
  const mode = useModeStore((state) => state.mode);

  // In local mode, always allow access (no auth required)
  if (mode === 'local') {
    return <>{children}</>;
  }

  return isAuthenticated ? <>{children}</> : <Navigate to="/login" />;
};

const AdminRoute = () => {
  const mode = useModeStore((state) => state.mode);
  const features = useModeStore((state) => state.features);

  // In local mode without user management, skip admin routes
  if (mode === 'local' && !features.userManagement) {
    return <Navigate to="/workspaces" replace />;
  }

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
    return <Navigate to="/workspaces" replace />;
  }

  return <Outlet />;
};

const ModeLoader = ({ children }: { children: React.ReactNode }) => {
  const mode = useModeStore((state) => state.mode);
  const fetchMode = useModeStore((state) => state.fetchMode);

  useEffect(() => {
    fetchMode();
  }, [fetchMode]);

  if (mode === 'loading') {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return <>{children}</>;
};

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ModeLoader>
        <BrowserRouter>
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route
              path="/"
              element={
                <PrivateRoute>
                  <Layout />
                </PrivateRoute>
              }
            >
              <Route index element={<Navigate to="/workspaces" replace />} />
              <Route path="workspaces" element={<Workspaces />} />
              <Route path="workspaces/:id" element={<WorkspaceDetail />} />
              <Route path="remote/workspaces/:id" element={<RemoteEnvironmentDetail />} />
              <Route path="settings" element={<Settings />} />
              <Route path="jobs" element={<Jobs />} />

              <Route element={<AdminRoute />}>
                <Route path="admin" element={<AdminDashboard />} />
                <Route path="admin/users" element={<UserManagement />} />
                <Route path="admin/audit-logs" element={<AuditLogs />} />
                <Route path="admin/registries" element={<RegistryManagement />} />
              </Route>
            </Route>
          </Routes>
        </BrowserRouter>
      </ModeLoader>
    </QueryClientProvider>
  );
}

export default App;
