import userEvent from '@testing-library/user-event';
import { type InitialEntry, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { configureOidc, resetOidc } from '@/lib/oidc';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';
import { CurrentPath } from '@/test/CurrentPath';
import { FakeUserManager, makeOidcUser } from '@/test/fakeOidc';
import { renderWithProviders, screen, waitFor } from '@/test/utils';
import { Login } from './Login';

vi.mock('oidc-client-ts', () => import('@/test/fakeOidc'));

const oidcConfig = {
  type: 'oidc' as const,
  issuer_url: 'https://auth.example.com/realms/nebi',
  client_id: 'nebi',
  scopes: ['openid'],
};

const renderLogin = (initialEntry: InitialEntry = '/login') =>
  renderWithProviders(
    <Routes>
      <Route path="/login" element={<Login isDarkMode={false} />} />
      <Route path="*" element={<CurrentPath />} />
    </Routes>,
    { initialEntries: [initialEntry] },
  );

const useOidc = () => {
  configureOidc(oidcConfig);
  useAuthStore.setState({
    config: oidcConfig,
    status: 'ready',
    oidcUser: null,
  });
  return FakeUserManager.latest();
};

describe('Login', () => {
  beforeEach(() => {
    resetOidc();
    FakeUserManager.reset();
    useModeStore.setState({ mode: 'team', loading: false });
  });

  afterEach(() => {
    useAuthStore.setState({ config: null, status: 'idle', oidcUser: null });
    useModeStore.setState({ mode: null });
  });

  it('starts the OIDC redirect from the Sign in button', async () => {
    const manager = useOidc();
    renderLogin();

    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    expect(manager.signinRedirect).toHaveBeenCalledWith({
      state: { returnTo: '/projects' },
    });
    expect(screen.queryByPlaceholderText('Password')).not.toBeInTheDocument();
  });

  it('returns to the page that required sign-in', async () => {
    const manager = useOidc();
    renderLogin({
      pathname: '/login',
      state: { from: '/projects/ws-1?tab=jobs' },
    });

    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    expect(manager.signinRedirect).toHaveBeenCalledWith({
      state: { returnTo: '/projects/ws-1?tab=jobs' },
    });
  });

  it('accepts a returnTo query parameter but not an off-site one', async () => {
    const manager = useOidc();
    renderLogin('/login?returnTo=%2F%2Fevil.example.com');

    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    expect(manager.signinRedirect).toHaveBeenCalledWith({
      state: { returnTo: '/projects' },
    });
  });

  it('shows a generic message for sign-in errors without echoing them', () => {
    useOidc();
    renderLogin('/login?error=Call%20IT%20at%201-800-555-0199');

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Sign in failed. Please try again.',
    );
    expect(
      screen.queryByText(/Call IT at 1-800-555-0199/),
    ).not.toBeInTheDocument();
  });

  it('reports when the identity provider cannot be reached', async () => {
    const manager = useOidc();
    manager.signinRedirect.mockRejectedValue(new Error('network'));
    renderLogin();

    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not reach the sign-in provider. Please try again.',
    );
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeEnabled();
  });

  it('skips the page when already signed in', async () => {
    useOidc();
    useAuthStore.setState({ oidcUser: makeOidcUser() });
    renderLogin({ pathname: '/login', state: { from: '/registries' } });

    expect(await screen.findByText('at /registries')).toBeInTheDocument();
  });

  it('goes straight to projects when the server has auth disabled', async () => {
    useAuthStore.setState({ config: { type: 'none' }, status: 'ready' });
    renderLogin();

    await waitFor(() =>
      expect(screen.getByText('at /projects')).toBeInTheDocument(),
    );
  });

  it('goes straight to projects in local mode', async () => {
    useModeStore.setState({ mode: 'local' });
    useAuthStore.setState({ config: { type: 'none' }, status: 'ready' });
    renderLogin();

    expect(await screen.findByText('at /projects')).toBeInTheDocument();
  });
});
