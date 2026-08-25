import { HttpResponse, http } from 'msw';
import { Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it } from 'vitest';
import { AdminRoute } from './App';
import { useModeStore } from './store/modeStore';
import { server } from './test/handlers';
import { renderWithProviders, screen } from './test/utils';

const renderAdminRoutes = () =>
  renderWithProviders(
    <Routes>
      <Route path="/workspaces" element={<div>workspaces page</div>} />
      <Route element={<AdminRoute />}>
        <Route path="/admin" element={<div>admin dashboard</div>} />
      </Route>
    </Routes>,
    { initialEntries: ['/admin'] },
  );

describe('AdminRoute', () => {
  afterEach(() => {
    useModeStore.setState({ mode: null, features: {}, loading: true });
  });

  it('redirects away from /admin in local mode without probing the admin API', async () => {
    useModeStore.setState({ mode: 'local', features: {}, loading: false });
    let probed = false;
    server.use(
      http.get('/api/v1/admin/users', () => {
        probed = true;
        return HttpResponse.json([]);
      }),
    );

    renderAdminRoutes();

    expect(await screen.findByText('workspaces page')).toBeInTheDocument();
    expect(screen.queryByText('admin dashboard')).not.toBeInTheDocument();
    expect(probed).toBe(false);
  });

  it('renders admin routes for admins in team mode', async () => {
    useModeStore.setState({ mode: 'team', features: {}, loading: false });

    renderAdminRoutes();

    expect(await screen.findByText('admin dashboard')).toBeInTheDocument();
  });

  it('redirects non-admins to workspaces in team mode', async () => {
    useModeStore.setState({ mode: 'team', features: {}, loading: false });
    server.use(
      http.get('/api/v1/admin/users', () =>
        HttpResponse.json({ error: 'Forbidden' }, { status: 403 }),
      ),
    );

    renderAdminRoutes();

    expect(await screen.findByText('workspaces page')).toBeInTheDocument();
    expect(screen.queryByText('admin dashboard')).not.toBeInTheDocument();
  });
});
