import { HttpResponse, http } from 'msw';
import { Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it } from 'vitest';
import { useAuthStore } from '@/store/authStore';
import { mockUser, server } from '@/test/handlers';
import { renderWithProviders, screen } from '@/test/utils';
import { Layout } from './Layout';

const renderLayout = () =>
  renderWithProviders(
    <Routes>
      <Route
        path="/"
        element={
          <Layout
            themeMode="light"
            isDarkMode={false}
            onThemeChange={() => {}}
          />
        }
      >
        <Route index element={<main>page</main>} />
      </Route>
    </Routes>,
  );

describe('server Layout', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: mockUser, token: 'tok' });
  });

  it('shows the server-only navigation from the prototype', async () => {
    renderLayout();
    expect(
      screen.getByRole('link', { name: 'Nebi server home' }),
    ).toBeInTheDocument();
    const nav = screen.getByRole('navigation', { name: 'Primary' });
    expect(nav).toHaveTextContent('Server projects');
    const docs = screen.getByRole('link', { name: /Docs/ });
    expect(docs).toHaveAttribute('target', '_blank');
    expect(
      screen.queryByRole('link', { name: /Registries|Settings/ }),
    ).toBeNull();
    expect(
      await screen.findByRole('link', { name: 'Admin' }),
    ).toBeInTheDocument();
  });

  it('hides the admin link from non-admins', async () => {
    server.use(
      http.get('/api/v1/admin/users', () =>
        HttpResponse.json({ error: 'Admin access required' }, { status: 403 }),
      ),
    );
    renderLayout();
    await screen.findByText('page');
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByRole('link', { name: 'Admin' })).toBeNull();
  });
});
