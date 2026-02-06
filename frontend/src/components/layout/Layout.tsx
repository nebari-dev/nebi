import { Outlet, NavLink, useNavigate } from 'react-router-dom';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';
import { useIsAdmin } from '@/hooks/useAdmin';
import { Button } from '@/components/ui/button';
import { LogOut, Boxes, ListTodo, Shield, Settings } from 'lucide-react';
import { useState } from 'react';

export const Layout = () => {
  const { user, clearAuth } = useAuthStore();
  const navigate = useNavigate();
  const { data: isAdmin } = useIsAdmin();
  const mode = useModeStore((state) => state.mode);
  const features = useModeStore((state) => state.features);
  const [avatarError, setAvatarError] = useState(false);

  const isLocal = mode === 'local';
  const showAdmin = !isLocal && isAdmin && features.userManagement;

  const handleLogout = () => {
    clearAuth();
    navigate('/login');
  };

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b bg-card">
        <div className="container mx-auto px-4 py-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-8">
              <img
                src="/nebi-logo.png"
                alt="Nebi"
                className="h-10 w-auto"
              />
              <nav className="flex gap-1">
                <NavLink to="/workspaces">
                  {({ isActive }) => (
                    <Button
                      variant={isActive ? 'secondary' : 'ghost'}
                      className="gap-2"
                    >
                      <Boxes className="h-4 w-4" />
                      Workspaces
                    </Button>
                  )}
                </NavLink>
                <NavLink to="/jobs">
                  {({ isActive }) => (
                    <Button
                      variant={isActive ? 'secondary' : 'ghost'}
                      className="gap-2"
                    >
                      <ListTodo className="h-4 w-4" />
                      Jobs
                    </Button>
                  )}
                </NavLink>
                {isLocal && (
                  <NavLink to="/settings">
                    {({ isActive }) => (
                      <Button
                        variant={isActive ? 'secondary' : 'ghost'}
                        className="gap-2"
                      >
                        <Settings className="h-4 w-4" />
                        Settings
                      </Button>
                    )}
                  </NavLink>
                )}
                {showAdmin && (
                  <NavLink to="/admin">
                    {({ isActive }) => (
                      <Button
                        variant={isActive ? 'secondary' : 'ghost'}
                        className="gap-2"
                      >
                        <Shield className="h-4 w-4" />
                        Admin
                      </Button>
                    )}
                  </NavLink>
                )}
              </nav>
            </div>
            {!isLocal && (
              <div className="flex items-center gap-4">
                {user?.avatar_url && !avatarError ? (
                  <img
                    src={user.avatar_url}
                    alt={user.username}
                    className="h-8 w-8 rounded-full"
                    referrerPolicy="no-referrer-when-downgrade"
                    crossOrigin="anonymous"
                    onError={() => setAvatarError(true)}
                  />
                ) : (
                  <div className="h-8 w-8 rounded-full bg-primary/10 flex items-center justify-center">
                    <span className="text-sm font-medium text-primary">
                      {user?.username?.charAt(0).toUpperCase()}
                    </span>
                  </div>
                )}
                <span className="text-sm font-medium text-foreground">
                  {user?.username}
                </span>
                <Button variant="ghost" size="icon" onClick={handleLogout}>
                  <LogOut className="h-4 w-4" />
                </Button>
              </div>
            )}
          </div>
        </div>
      </header>
      <main className="container mx-auto px-4 py-8">
        <Outlet />
      </main>
    </div>
  );
};
