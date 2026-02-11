import { useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate, Outlet } from 'react-router-dom';
import { QueryClientProvider, useQuery } from '@tanstack/react-query';
import { queryClient } from './lib/queryClient';
import { useAuthStore } from './store/authStore';
import { useModeStore } from './store/modeStore';
import { Login } from './pages/Login';
import { Workspaces } from './pages/Workspaces';
import { WorkspaceDetail } from './pages/WorkspaceDetail';
import { RemoteWorkspaceDetail } from './pages/RemoteWorkspaceDetail';
import { Jobs } from './pages/Jobs';
import { Settings } from './pages/Settings';
import { AdminDashboard } from './pages/admin/AdminDashboard';
import { UserManagement } from './pages/admin/UserManagement';
import { AuditLogs } from './pages/admin/AuditLogs';
import { RegistryManagement } from './pages/admin/RegistryManagement';
import { Layout } from './components/layout/Layout';
import { AdminLayout } from './components/layout/AdminLayout';
import { adminApi } from './api/admin';
import { Loader2 } from 'lucide-react';

// Load mode before rendering any routes
const ModeLoader = ({ children }: { children: React.ReactNode }) => {
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
  return <>{children}</>;
};

const PrivateRoute = ({ children }: { children: React.ReactNode }) => {
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated());
  const isLocalMode = useModeStore((state) => state.isLocalMode());

  // In local mode, auth is bypassed
  if (isLocalMode) return <>{children}</>;

  return isAuthenticated ? <>{children}</> : <Navigate to="/login" />;
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
    return <Navigate to="/workspaces" replace />;
  }

  return <Outlet />;
};

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <ModeLoader>
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
              <Route path="remote/workspaces/:id" element={<RemoteWorkspaceDetail />} />
              <Route path="jobs" element={<Jobs />} />
              <Route path="settings" element={<Settings />} />

              <Route element={<AdminRoute />}>
                <Route element={<AdminLayout />}>
                  <Route path="admin" element={<AdminDashboard />} />
                  <Route path="admin/users" element={<UserManagement />} />
                  <Route path="admin/audit-logs" element={<AuditLogs />} />
                  <Route path="admin/registries" element={<RegistryManagement />} />
                </Route>
              </Route>
            </Route>
          </Routes>
        </ModeLoader>
      </BrowserRouter>
    </QueryClientProvider>
  );
}

export default App;
