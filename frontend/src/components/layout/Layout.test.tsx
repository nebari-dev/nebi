import { HttpResponse, http } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';
import { mockUser, server } from '@/test/handlers';
import { renderWithProviders, screen } from '@/test/utils';
import { Layout } from './Layout';

const renderLayout = () =>
  renderWithProviders(
    <Layout themeMode="system" isDarkMode={false} onThemeChange={vi.fn()} />,
  );

const useDisconnectedRemoteServer = () =>
  server.use(
    http.get('/api/v1/remote/server', () =>
      HttpResponse.json({ url: '', username: '', status: 'disconnected' }),
    ),
  );

describe('Layout', () => {
  afterEach(() => {
    useModeStore.setState({ mode: null, features: {}, loading: true });
    useAuthStore.setState({ token: null, user: null });
  });

  it('hides the Admin button and profile menu in local mode', async () => {
    useModeStore.setState({ mode: 'local', features: {}, loading: false });
    // Even with a stale user in the auth store, local mode shows neither.
    useAuthStore.setState({ token: 'test-token', user: mockUser });
    useDisconnectedRemoteServer();

    renderLayout();

    // Wait for the version footer so the header has fully settled.
    expect(await screen.findByText('v0.0.1')).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /admin/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /testuser/i }),
    ).not.toBeInTheDocument();
  });

  it('shows the Admin button and profile menu for admins in team mode', async () => {
    useModeStore.setState({ mode: 'team', features: {}, loading: false });
    useAuthStore.setState({ token: 'test-token', user: mockUser });
    useDisconnectedRemoteServer();

    renderLayout();

    // The default /admin/users handler succeeds, so useIsAdmin resolves true.
    expect(
      await screen.findByRole('button', { name: /admin/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /testuser/i }),
    ).toBeInTheDocument();
  });
});
