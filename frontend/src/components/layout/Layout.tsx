import { Boxes, ExternalLink, Settings, Shield } from 'lucide-react';
import {
  Outlet,
  NavLink as RouterNavLink,
  useLocation,
  useNavigate,
} from 'react-router-dom';
import { Button } from '@/components/ui/button';
import {
  MenuBarActions,
  MenuBarBrand,
  MenuBarNav,
  NavigationMenu,
  NavLink,
} from '@/components/ui/navigation-menu';
import { useIsAdmin } from '@/hooks/useAdmin';
import { useHostJobNotifications } from '@/hooks/useHostJobNotifications';
import { useRemoteView } from '@/hooks/useRemote';
import type { ThemeMode } from '@/hooks/useThemePreference';
import { useVersion } from '@/hooks/useVersion';
import { getBrandingLogoUrl } from '@/lib/brandingConfig';
import { openExternal } from '@/lib/openExternal';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';
import { ProfileMenu } from './ProfileMenu';

type LayoutProps = {
  themeMode: ThemeMode;
  isDarkMode: boolean;
  onThemeChange: (themeMode: ThemeMode) => void;
};

const registriesIcon = (
  <svg
    aria-hidden="true"
    xmlns="http://www.w3.org/2000/svg"
    width="16"
    height="16"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    className="h-4 w-4"
  >
    <path d="M11.5 20h-6.5a2 2 0 0 1 -2 -2v-12a2 2 0 0 1 2 -2h14a2 2 0 0 1 2 2v5.5" />
    <path d="M9 17h2" />
    <path d="M18 18m-3 0a3 3 0 1 0 6 0a3 3 0 1 0 -6 0" />
    <path d="M20.2 20.2l1.8 1.8" />
  </svg>
);

export const Layout = ({
  themeMode,
  isDarkMode,
  onThemeChange,
}: LayoutProps) => {
  const { user, clearAuth } = useAuthStore();
  const navigate = useNavigate();
  const { data: isAdmin } = useIsAdmin();
  const { setViewMode } = useViewModeStore();
  const { data: versionInfo } = useVersion();
  // Layout never unmounts, so this call keeps a permanent observer on the
  // remote/server query — the app's only connection-status self-heal (see the
  // useRemoteServer comment in hooks/useRemote.ts).
  const { isLocalMode, viewMode, isRemoteConnected } = useRemoteView();
  useHostJobNotifications();

  const location = useLocation();
  const isAdminPage = location.pathname.startsWith('/admin');

  const logoutUrl = useModeStore((s) => s.logoutUrl);

  const handleLogout = () => {
    clearAuth();
    if (logoutUrl) {
      // Signal Login.tsx to NOT auto-redirect back to /auth/session
      sessionStorage.setItem('nebi_logout', '1');
      // Redirect to the gateway's OIDC logout path (e.g. Envoy's /logout)
      // to clear IdToken cookies and terminate the Keycloak session.
      window.location.href = logoutUrl;
    } else {
      navigate('/login');
    }
  };

  return (
    <div className="min-h-screen bg-canvas flex flex-col">
      <NavigationMenu className="h-14 shrink-0 justify-between border-border bg-header pl-4 text-header-foreground">
        <MenuBarBrand
          href="/workspaces"
          aria-label="Go to workspaces"
          onClick={(event) => {
            event.preventDefault();
            navigate('/workspaces');
          }}
        >
          <img
            src={getBrandingLogoUrl(isDarkMode)}
            alt="Nebi"
            className="h-8 w-auto"
          />
        </MenuBarBrand>
        <MenuBarNav aria-label="Primary" className="ml-4">
          <NavLink
            render={<RouterNavLink to="/workspaces" />}
            active={location.pathname === '/workspaces'}
            icon={<Boxes className="h-4 w-4" />}
          >
            Workspaces
          </NavLink>
          <NavLink
            render={<RouterNavLink to="/registries" />}
            active={location.pathname === '/registries'}
            icon={registriesIcon}
          >
            Registries
          </NavLink>
          {isLocalMode && (
            <NavLink
              render={<RouterNavLink to="/settings" />}
              active={location.pathname === '/settings'}
              icon={<Settings className="h-4 w-4" />}
            >
              Settings
            </NavLink>
          )}
        </MenuBarNav>
        <MenuBarActions className="gap-2">
          {/* View Mode Toggle - only show when remote is connected */}
          {isRemoteConnected && (
            <div className="flex items-center gap-0.5 p-[3px] bg-muted rounded-lg border border-border">
              <button
                type="button"
                onClick={() => setViewMode('local')}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-all ${
                  viewMode === 'local'
                    ? 'bg-card text-foreground shadow-sm'
                    : 'text-muted-foreground-strong hover:text-foreground'
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
                type="button"
                onClick={() => setViewMode('remote')}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-all ${
                  viewMode === 'remote'
                    ? 'bg-card text-foreground shadow-sm'
                    : 'text-muted-foreground-strong hover:text-foreground'
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
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              aria-label="Docs"
              title="Docs"
              className="hover:bg-header-action-hover hover:no-underline active:bg-header-action-hover"
              onClick={() => openExternal('https://nebi.nebari.dev/')}
            >
              <ExternalLink className="h-4 w-4" />
            </Button>
            {isAdmin && (
              <Button
                render={<RouterNavLink to="/admin" />}
                variant={isAdminPage ? 'secondary' : 'ghost'}
                size="icon"
                aria-label="Admin"
                title="Admin"
                className="hover:bg-header-action-hover hover:no-underline active:bg-header-action-hover"
              >
                <Shield className="h-4 w-4" />
              </Button>
            )}
            {!isLocalMode && (
              <ProfileMenu
                user={user}
                themeMode={themeMode}
                onThemeChange={onThemeChange}
                onLogout={handleLogout}
              />
            )}
          </div>
        </MenuBarActions>
      </NavigationMenu>
      <main
        aria-label={isAdminPage ? 'Admin content' : 'Main content'}
        className={isAdminPage ? 'flex-1 overflow-hidden' : 'flex-1 px-12 py-6'}
      >
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
            className="text-sm text-muted-foreground hover:text-muted-foreground transition-colors"
          >
            v{versionInfo.version}
          </a>
        </footer>
      )}
    </div>
  );
};
