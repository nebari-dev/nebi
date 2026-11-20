import { HttpResponse, http, type JsonBodyType } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { getUserManager, resetOidc } from '@/lib/oidc';
import { FakeUserManager, makeOidcUser } from '@/test/fakeOidc';
import { mockUser, server } from '@/test/handlers';
import { useAuthStore } from './authStore';

vi.mock('oidc-client-ts', () => import('@/test/fakeOidc'));

const oidcConfig = {
  type: 'oidc' as const,
  issuer_url: 'https://auth.example.com/realms/nebi',
  client_id: 'nebi',
  scopes: ['openid', 'profile', 'email'],
};

const serveAuthConfig = (body: JsonBodyType, status = 200) =>
  server.use(
    http.get('/api/v1/auth/config', () => HttpResponse.json(body, { status })),
  );

beforeEach(() => {
  resetOidc();
  FakeUserManager.reset();
  useAuthStore.setState({
    config: null,
    status: 'idle',
    oidcUser: null,
    user: null,
  });
});

afterEach(() => {
  window.__NEBI_BASE_PATH__ = undefined;
});

describe('initialize', () => {
  it('needs no authentication in local mode', async () => {
    await useAuthStore.getState().initialize('local');

    const state = useAuthStore.getState();
    expect(state.status).toBe('ready');
    expect(state.config).toEqual({ type: 'none' });
    expect(state.isOidc()).toBe(false);
    expect(state.isAuthenticated()).toBe(true);
    expect(getUserManager()).toBeNull();
  });

  it('treats a team server with auth disabled as always authenticated', async () => {
    serveAuthConfig({ type: 'none' });

    await useAuthStore.getState().initialize('team');

    const state = useAuthStore.getState();
    expect(state.config).toEqual({ type: 'none' });
    expect(state.isAuthenticated()).toBe(true);
    expect(getUserManager()).toBeNull();
  });

  it('configures an OIDC public client from the server auth config', async () => {
    window.__NEBI_BASE_PATH__ = '/nebi';
    serveAuthConfig(oidcConfig);

    await useAuthStore.getState().initialize('team');

    const manager = FakeUserManager.latest();
    expect(getUserManager()).toBe(manager);
    expect(manager.settings).toMatchObject({
      authority: oidcConfig.issuer_url,
      client_id: 'nebi',
      redirect_uri: `${window.location.origin}/nebi/auth/callback`,
      post_logout_redirect_uri: `${window.location.origin}/nebi/login`,
      response_type: 'code',
      scope: 'openid profile email',
      automaticSilentRenew: true,
    });
    const state = useAuthStore.getState();
    expect(state.isOidc()).toBe(true);
    expect(state.isAuthenticated()).toBe(false);
  });

  it('restores a stored, unexpired session', async () => {
    serveAuthConfig(oidcConfig);
    const user = makeOidcUser();
    FakeUserManager.onCreate = (manager) => {
      manager.user = user;
    };

    await useAuthStore.getState().initialize('team');

    expect(useAuthStore.getState().oidcUser).toBe(user);
    expect(useAuthStore.getState().isAuthenticated()).toBe(true);
  });

  it('refreshes an expired session before deciding the user is signed out', async () => {
    serveAuthConfig(oidcConfig);
    const expired = makeOidcUser({ expired: true });
    const renewed = makeOidcUser({ access_token: 'renewed' });
    FakeUserManager.onCreate = (manager) => {
      manager.user = expired;
      manager.signinSilent.mockResolvedValue(renewed);
    };

    await useAuthStore.getState().initialize('team');

    expect(useAuthStore.getState().oidcUser).toBe(renewed);
    expect(useAuthStore.getState().isAuthenticated()).toBe(true);
  });

  it('drops an expired session that cannot be refreshed', async () => {
    serveAuthConfig(oidcConfig);
    FakeUserManager.onCreate = (manager) => {
      manager.user = makeOidcUser({ expired: true, refresh_token: undefined });
    };

    await useAuthStore.getState().initialize('team');

    const manager = FakeUserManager.latest();
    expect(manager.removeUser).toHaveBeenCalled();
    expect(useAuthStore.getState().oidcUser).toBeNull();
    expect(useAuthStore.getState().isAuthenticated()).toBe(false);
  });

  it('reports an error when the auth config cannot be loaded', async () => {
    serveAuthConfig({ error: 'boom' }, 500);

    await useAuthStore.getState().initialize('team');

    expect(useAuthStore.getState().status).toBe('error');
  });

  it('fails closed on an auth type it does not understand', async () => {
    serveAuthConfig({ type: 'saml' });

    await useAuthStore.getState().initialize('team');

    expect(useAuthStore.getState().status).toBe('error');
    expect(useAuthStore.getState().isAuthenticated()).toBe(false);
  });

  it('only initializes once', async () => {
    serveAuthConfig(oidcConfig);

    await Promise.all([
      useAuthStore.getState().initialize('team'),
      useAuthStore.getState().initialize('team'),
    ]);

    expect(FakeUserManager.instances).toHaveLength(1);
  });
});

describe('session events', () => {
  it('tracks sign-in and sign-out from the UserManager', async () => {
    serveAuthConfig(oidcConfig);
    await useAuthStore.getState().initialize('team');
    const manager = FakeUserManager.latest();
    const user = makeOidcUser();

    manager.load(user);
    expect(useAuthStore.getState().oidcUser).toBe(user);
    expect(useAuthStore.getState().isAuthenticated()).toBe(true);

    useAuthStore.getState().setUser(mockUser);
    await manager.removeUser();
    expect(useAuthStore.getState().oidcUser).toBeNull();
    expect(useAuthStore.getState().user).toBeNull();
    expect(useAuthStore.getState().isAuthenticated()).toBe(false);
  });

  it('does not count an expired access token as authenticated', async () => {
    serveAuthConfig(oidcConfig);
    await useAuthStore.getState().initialize('team');

    FakeUserManager.latest().load(makeOidcUser({ expired: true }));

    expect(useAuthStore.getState().isAuthenticated()).toBe(false);
  });
});

describe('clearAuth', () => {
  it('clears the session and the Nebi user', () => {
    useAuthStore.setState({ oidcUser: makeOidcUser(), user: mockUser });

    useAuthStore.getState().clearAuth();

    expect(useAuthStore.getState().oidcUser).toBeNull();
    expect(useAuthStore.getState().user).toBeNull();
  });
});
