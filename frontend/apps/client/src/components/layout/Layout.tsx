// App shell for the client (issue #649), matching the prototype header:
// logo + "Nebi", nav Projects / Server / Registries / Jobs, then the
// connected-server badge, notifications bell, help menu and theme menu.
// Frozen: the slot components live in unit-owned files
// (ConnectedServerBadge: U5, NotificationsBell: U8, HelpMenu: U2).
// Pages render their own <main>.
import { ThemeMenu } from '@nebi/ui/components/theme-menu';
import { Button } from '@nebi/ui/components/ui/button';
import {
  MenuBarActions,
  MenuBarBrand,
  MenuBarNav,
  NavigationMenu,
  NavLink,
} from '@nebi/ui/components/ui/navigation-menu';
import { useTheme } from '@nebi/ui/hooks/theme-provider';
import { cn } from '@nebi/ui/lib/utils';
import { Boxes, Package, RotateCcwClock, Server, Shield } from 'lucide-react';
import {
  Outlet,
  NavLink as RouterNavLink,
  useLocation,
  useNavigate,
} from 'react-router-dom';
import { useIsAdmin } from '@/hooks/useAdmin';
import { useHostJobNotifications } from '@/hooks/useHostJobNotifications';
import { getBrandingLogoUrl } from '@/lib/brandingConfig';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';
import { ConnectedServerBadge } from './ConnectedServerBadge';
import { HelpMenu } from './HelpMenu';
import { NotificationsBell } from './NotificationsBell';
import { ProfileMenu } from './ProfileMenu';

const navItemClass = 'h-full rounded-none';
const activeUnderline =
  "after:absolute after:right-2 after:bottom-0 after:left-2 after:h-0.5 after:bg-primary after:content-['']";

export const Layout = () => {
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const { themeMode, isDarkMode, setThemeMode } = useTheme();
  const { user, clearAuth } = useAuthStore();
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const logoutUrl = useModeStore((s) => s.logoutUrl);
  const { data: isAdmin } = useIsAdmin();
  useHostJobNotifications();

  const inProjects = pathname === '/' || pathname.startsWith('/projects');
  const isAdminPage = pathname.startsWith('/admin');

  const handleLogout = () => {
    clearAuth();
    if (logoutUrl) {
      sessionStorage.setItem('nebi_logout', '1');
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
          aria-label="Nebi home"
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
            icon={<Boxes />}
            active={pathname === '/'}
            render={<RouterNavLink to="/" end />}
            className={cn(
              navItemClass,
              inProjects && pathname !== '/' && activeUnderline,
            )}
          >
            Projects
          </NavLink>
          <NavLink
            icon={<Server />}
            active={pathname === '/server'}
            render={<RouterNavLink to="/server" />}
            className={navItemClass}
          >
            Server
          </NavLink>
          <NavLink
            icon={<Package />}
            active={pathname === '/registries'}
            render={<RouterNavLink to="/registries" />}
            className={navItemClass}
          >
            Registries
          </NavLink>
          <NavLink
            icon={<RotateCcwClock />}
            active={pathname === '/jobs'}
            render={<RouterNavLink to="/jobs" />}
            className={cn(
              navItemClass,
              pathname.startsWith('/jobs/') && activeUnderline,
            )}
          >
            Jobs
          </NavLink>
        </MenuBarNav>
        <MenuBarActions className="gap-2">
          <ConnectedServerBadge />
          <NotificationsBell />
          <HelpMenu />
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
          {isLocalMode ? (
            <ThemeMenu />
          ) : (
            <ProfileMenu
              user={user}
              themeMode={themeMode}
              onThemeChange={setThemeMode}
              onLogout={handleLogout}
            />
          )}
        </MenuBarActions>
      </NavigationMenu>
      <div className="flex flex-1 flex-col">
        <Outlet />
      </div>
    </div>
  );
};
