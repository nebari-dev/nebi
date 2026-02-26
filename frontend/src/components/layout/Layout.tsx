import { Outlet, NavLink, useNavigate } from 'react-router-dom';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';
import { getBasePath } from '@/lib/basePath';
import { useViewModeStore } from '@/store/viewModeStore';
import { useIsAdmin } from '@/hooks/useAdmin';
import { useRemoteServer } from '@/hooks/useRemote';
import { useVersion } from '@/hooks/useVersion';
import { Button } from '@/components/ui/button';

import { LogOut, Boxes, ListTodo, Shield, Settings } from 'lucide-react';
import { useState } from 'react';

export const Layout = () => {
  const { user, clearAuth } = useAuthStore();
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const navigate = useNavigate();
  const { data: isAdmin } = useIsAdmin();
  const [avatarError, setAvatarError] = useState(false);
  const { viewMode, setViewMode } = useViewModeStore();
  const { data: serverStatus } = useRemoteServer();
  const { data: versionInfo } = useVersion();
  const isRemoteConnected = isLocalMode && serverStatus?.status === 'connected';

  const handleLogout = () => {
    clearAuth();
    navigate('/login');
  };

  return (
    <div className="min-h-screen bg-background flex flex-col">
      <header className="border-b bg-card">
        <div className="container mx-auto px-4 py-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-8">
              <NavLink to="/workspaces">
                <img
                  src={`${getBasePath()}/nebi-logo.png`}
                  alt="Nebi"
                  className="h-10 w-auto"
                />
              </NavLink>
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
                <NavLink to="/registries">
                  {({ isActive }) => (
                    <Button
                      variant={isActive ? 'secondary' : 'ghost'}
                      className="gap-2"
                    >
                      <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="h-4 w-4"><path d="M11.5 20h-6.5a2 2 0 0 1 -2 -2v-12a2 2 0 0 1 2 -2h14a2 2 0 0 1 2 2v5.5" /><path d="M9 17h2" /><path d="M18 18m-3 0a3 3 0 1 0 6 0a3 3 0 1 0 -6 0" /><path d="M20.2 20.2l1.8 1.8" /></svg>
                      Registries
                    </Button>
                  )}
                </NavLink>
                {isLocalMode && (
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
              </nav>
            </div>
            <div className="flex items-center gap-4">
              {/* View Mode Toggle - only show when remote is connected */}
              {isRemoteConnected && (
                <div className="flex items-center gap-0.5 p-[3px] bg-muted rounded-lg border border-border">
                  <button
                    onClick={() => setViewMode('local')}
                    className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-all ${
                      viewMode === 'local'
                        ? 'bg-white text-foreground shadow-sm'
                        : 'text-muted-foreground hover:text-foreground'
                    }`}
                  >
                    <span
                      className={`w-1.5 h-1.5 rounded-full transition-all ${
                        viewMode === 'local'
                          ? 'bg-primary shadow-[0_0_6px_rgba(155,61,204,0.4)]'
                          : 'bg-muted-foreground/50'
                      }`}
                    />
                    Local
                  </button>
                  <button
                    onClick={() => setViewMode('remote')}
                    className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-all ${
                      viewMode === 'remote'
                        ? 'bg-white text-foreground shadow-sm'
                        : 'text-muted-foreground hover:text-foreground'
                    }`}
                  >
                    <span
                      className={`w-1.5 h-1.5 rounded-full transition-all ${
                        viewMode === 'remote'
                          ? 'bg-primary shadow-[0_0_6px_rgba(155,61,204,0.4)]'
                          : 'bg-muted-foreground/50'
                      }`}
                    />
                    Remote
                  </button>
                </div>
              )}
              {isAdmin && (
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
              {!isLocalMode && (
                <>
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
                </>
              )}
            </div>
          </div>
        </div>
      </header>
      <main className="container mx-auto px-4 py-8 flex-1">
        <Outlet />
      </main>
      {versionInfo?.version && (
        <footer className="border-t border-border/60 py-4 px-8">
          <a
            href={
              versionInfo.commit
                ? `https://github.com/nebari-dev/nebi/commit/${versionInfo.commit}`
                : `https://github.com/nebari-dev/nebi/releases/tag/v${versionInfo.version}`
            }
            target="_blank"
            rel="noopener noreferrer"
            className="text-sm text-muted-foreground/60 hover:text-muted-foreground transition-colors"
          >
            v{versionInfo.version}
          </a>
        </footer>
      )}
    </div>
  );
};
