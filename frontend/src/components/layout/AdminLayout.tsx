import { Outlet, NavLink } from 'react-router-dom';
import { LayoutDashboard, Users, Package, Activity } from 'lucide-react';

const navItems = [
  { to: '/admin', label: 'Overview', icon: LayoutDashboard, end: true },
  { to: '/admin/users', label: 'Users', icon: Users, end: false },
  { to: '/admin/registries', label: 'Registries', icon: Package, end: false },
  { to: '/admin/audit-logs', label: 'Logs', icon: Activity, end: false },
];

export const AdminLayout = () => {
  return (
    <div className="flex min-h-[calc(100vh-73px)]">
      <aside className="w-[253px] shrink-0 border-r bg-card px-4 py-6">
        <div className="mb-6">
          <h2 className="text-lg font-semibold">Admin Dashboard</h2>
          <p className="text-sm text-muted-foreground">
            System overview and management
          </p>
        </div>
        <nav className="flex flex-col gap-1">
          {navItems.map(({ to, label, icon: Icon, end }) => (
            <NavLink key={to} to={to} end={end}>
              {({ isActive }) => (
                <div
                  className={`flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                    isActive
                      ? 'bg-[#E8D7FB] text-[#9B3DCC]'
                      : 'text-muted-foreground hover:bg-[var(--color-nav-hover)] hover:text-foreground'
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
      <main className="flex-1 overflow-auto p-8">
        <Outlet />
      </main>
    </div>
  );
};
