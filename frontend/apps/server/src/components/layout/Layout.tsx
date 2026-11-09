// App shell of the team server UI, matching the prototype ServerUi header:
// logo + "Nebi", nav "Server projects" and Docs, then the server badge, the
// admin shield (admins) and the profile menu. Pages render their own
// <main>; admin pages are wrapped in one here.
import { Badge } from '@nebi/ui/components/ui/badge';
import { Button } from '@nebi/ui/components/ui/button';
import {
  MenuBarActions,
  MenuBarBrand,
  MenuBarNav,
  NavigationMenu,
  NavLink,
} from '@nebi/ui/components/ui/navigation-menu';
import type { ThemeMode } from '@nebi/ui/hooks/use-theme-preference';
import { ExternalLink, Server, Shield } from 'lucide-react';
import {
  Outlet,
  NavLink as RouterNavLink,
  useLocation,
  useNavigate,
} from 'react-router-dom';
import { useIsAdmin } from '@/hooks/useAdmin';
import { getBrandingLogoUrl } from '@/lib/brandingConfig';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';
import { ProfileMenu } from './ProfileMenu';

type LayoutProps = {
  themeMode: ThemeMode;
  isDarkMode: boolean;
  onThemeChange: (themeMode: ThemeMode) => void;
};

const navItemClass = 'h-full rounded-none';

export const Layout = ({
  themeMode,
  isDarkMode,
  onThemeChange,
}: LayoutProps) => {
  const { user, clearAuth } = useAuthStore();
  const navigate = useNavigate();
  const { data: isAdmin } = useIsAdmin();
  const { pathname } = useLocation();
  const isAdminPage = pathname.startsWith('/admin');
  const isLocalMode = useModeStore((s) => s.isLocalMode());
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
    <div className="flex min-h-screen flex-col bg-canvas">
      <NavigationMenu className="h-14 shrink-0 gap-3 border-border bg-header pl-4 text-header-foreground">
        <MenuBarBrand
          href="/"
          aria-label="Nebi server home"
          onClick={(event) => {
            event.preventDefault();
            navigate('/');
          }}
        >
          {/* The default logo already contains the "Nebi" wordmark. */}
          <img
            src={getBrandingLogoUrl(isDarkMode)}
            alt="Nebi"
            className="h-8 w-auto"
          />
        </MenuBarBrand>
        <MenuBarNav aria-label="Primary" className="self-stretch">
          <NavLink
            icon={<Server />}
            active={pathname === '/'}
            render={<RouterNavLink to="/" end />}
            className={navItemClass}
          >
            Server projects
          </NavLink>
          <NavLink
            render={
              <a
                href="https://nebi.nebari.dev/"
                target="_blank"
                rel="noreferrer"
              />
            }
            className={navItemClass}
          >
            <span className="inline-flex items-center gap-1.5">
              Docs
              <ExternalLink className="size-4" aria-hidden />
              <span className="sr-only">(opens in a new tab)</span>
            </span>
          </NavLink>
        </MenuBarNav>
        <MenuBarActions className="gap-2">
          <Badge
            variant="outline"
            className="border-border font-medium text-foreground"
          >
            <Server aria-hidden />
            {window.location.host}
          </Badge>
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
        </MenuBarActions>
      </NavigationMenu>
      {isAdminPage ? (
        <main aria-label="Admin content" className="flex-1 overflow-hidden">
          <Outlet />
        </main>
      ) : (
        <div className="flex flex-1 flex-col">
          <Outlet />
        </div>
      )}
    </div>
  );
};
