import {
  Activity,
  Fingerprint,
  LayoutDashboard,
  Package,
  Users,
  Users2,
} from 'lucide-react';
import { NavLink, Outlet } from 'react-router-dom';

const navItems = [
  { to: '/admin', label: 'Overview', icon: LayoutDashboard, end: true },
  { to: '/admin/users', label: 'Users', icon: Users, end: false },
  { to: '/admin/groups', label: 'Groups', icon: Users2, end: false },
  {
    to: '/admin/identity-reviews',
    label: 'Identity Reviews',
    icon: Fingerprint,
    end: false,
  },
  { to: '/admin/registries', label: 'Registries', icon: Package, end: false },
  { to: '/admin/audit-logs', label: 'Logs', icon: Activity, end: false },
];

export const AdminLayout = () => {
  return (
    <main aria-label="Admin content" className="flex h-[calc(100vh-56px)]">
      <aside className="w-[253px] shrink-0 border-r bg-sidebar py-6 text-sidebar-foreground">
        <div className="mb-6 px-4">
          <h2 className="text-lg font-semibold">Admin Dashboard</h2>
          <p className="text-sm text-muted-foreground">
            System overview and management
          </p>
        </div>
        <nav className="flex flex-col gap-1 px-2" aria-label="Admin">
          {navItems.map(({ to, label, icon: Icon, end }) => (
            <NavLink key={to} to={to} end={end} className="block w-full">
              {({ isActive }) => (
                <div
                  className={`flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                    isActive
                      ? 'bg-primary/10 text-foreground'
                      : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                  }`}
                >
                  <Icon className="h-4 w-4" />
                  {label}
                </div>
              )}
            </NavLink>
          ))}
        </nav>
      </aside>
      <section
        className="flex-1 overflow-auto bg-canvas px-12 py-6"
        aria-label="Admin page"
      >
        <Outlet />
      </section>
    </main>
  );
};
