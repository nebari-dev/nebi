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
  useLocation,
} from 'react-router-dom';
import { adminApi } from './api/admin';
import { AdminLayout } from './components/layout/AdminLayout';
import { Layout } from './components/layout/Layout';
import { Button } from './components/ui/button';
import { useTheme } from './hooks/theme-provider';
import { getBasePath } from './lib/basePath';
import { queryClient } from './lib/queryClient';
import { AuthCallback } from './pages/AuthCallback';
import { AdminDashboard } from './pages/admin/AdminDashboard';
import { AuditLogs } from './pages/admin/AuditLogs';
import { Groups } from './pages/admin/Groups';
import { RegistryManagement } from './pages/admin/RegistryManagement';
import { UserManagement } from './pages/admin/UserManagement';
import { Login } from './pages/Login';
import { ProjectDetail } from './pages/ProjectDetail';
import { Projects } from './pages/Projects';
import { Registries, RegistryRepositories } from './pages/Registries';
import { RemoteProjectDetail } from './pages/RemoteProjectDetail';
import { Settings } from './pages/Settings';
import { useAuthStore } from './store/authStore';
import { useModeStore } from './store/modeStore';

// Load mode, then the server's auth config, before rendering any routes
const ModeLoader = ({ children }: { children: ReactElement }) => {
  const { loading, mode, fetchMode } = useModeStore();
  const authStatus = useAuthStore((state) => state.status);
  const initializeAuth = useAuthStore((state) => state.initialize);

  useEffect(() => {
    fetchMode();
  }, [fetchMode]);

  useEffect(() => {
    if (!loading && mode) {
      void initializeAuth(mode);
    }
  }, [loading, mode, initializeAuth]);

  if (authStatus === 'error') {
    return (
      <div className="flex h-screen flex-col items-center justify-center gap-4">
        <p className="text-muted-foreground">
          Could not load the server's sign-in configuration.
        </p>
        <Button variant="outline" onClick={() => mode && initializeAuth(mode)}>
          Retry
        </Button>
      </div>
    );
  }

  if (loading || authStatus !== 'ready') {
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
  const location = useLocation();

  // In local mode, auth is bypassed
  if (isLocalMode) return children;

  return isAuthenticated ? (
    children
  ) : (
    <Navigate
      to="/login"
      replace
      state={{ from: location.pathname + location.search }}
    />
  );
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
    return <Navigate to="/projects" replace />;
  }

  return <Outlet />;
};

function App() {
  const { themeMode, isDarkMode, setThemeMode } = useTheme();

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter basename={getBasePath()}>
        <ModeLoader>
          <Routes>
            <Route path="/login" element={<Login isDarkMode={isDarkMode} />} />
            <Route path="/auth/callback" element={<AuthCallback />} />
            <Route
              path="/"
              element={
                <PrivateRoute>
                  <Layout
                    themeMode={themeMode}
                    isDarkMode={isDarkMode}
                    onThemeChange={setThemeMode}
                  />
                </PrivateRoute>
              }
            >
              <Route index element={<Navigate to="/projects" replace />} />
              <Route path="projects" element={<Projects />} />
              <Route path="projects/:id" element={<ProjectDetail />} />
              <Route
                path="remote/projects/:id"
                element={<RemoteProjectDetail />}
              />
              <Route path="registries" element={<Registries />} />
              <Route
                path="registries/:registryId"
                element={<RegistryRepositories />}
              />
              <Route path="settings" element={<Settings />} />

              <Route element={<AdminRoute />}>
                <Route element={<AdminLayout />}>
                  <Route path="admin" element={<AdminDashboard />} />
                  <Route path="admin/users" element={<UserManagement />} />
                  <Route path="admin/groups" element={<Groups />} />
                  <Route path="admin/audit-logs" element={<AuditLogs />} />
                  <Route
                    path="admin/registries"
                    element={<RegistryManagement />}
                  />
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
