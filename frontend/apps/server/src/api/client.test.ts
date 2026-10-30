import { HttpResponse, http } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { configureOidc, resetOidc } from '@/lib/oidc';
import { queryClient } from '@/lib/queryClient';
import { useModeStore } from '@/store/modeStore';
import { FakeUserManager, makeOidcUser } from '@/test/fakeOidc';
import { server } from '@/test/handlers';
import { apiClient } from './client';

vi.mock('oidc-client-ts', () => import('@/test/fakeOidc'));

const oidcConfig = {
  type: 'oidc' as const,
  issuer_url: 'https://auth.example.com/realms/nebi',
  client_id: 'nebi',
  scopes: ['openid'],
};

// Records the Authorization header of every request to /protected-resource
// and answers with the given statuses in order (200 once they run out).
const serveProtected = (statuses: number[] = []) => {
  const seen: Array<string | null> = [];
  server.use(
    http.get('/api/v1/protected-resource', ({ request }) => {
      seen.push(request.headers.get('Authorization'));
      const status = statuses.shift() ?? 200;
      return HttpResponse.json(
        status === 200 ? { ok: true } : { error: 'unauthorized' },
        { status },
      );
    }),
  );
  return seen;
};

const stubLocation = (pathname = '/projects/ws-1', search = '?tab=jobs') => {
  const location = {
    ...window.location,
    href: window.location.href,
    pathname,
    search,
  };
  vi.spyOn(window, 'location', 'get').mockReturnValue(location as Location);
  return location;
};

beforeEach(() => {
  resetOidc();
  FakeUserManager.reset();
  useModeStore.setState({ mode: 'team', features: {}, loading: false });
});

afterEach(() => {
  vi.restoreAllMocks();
  resetOidc();
});

describe('apiClient request interceptor', () => {
  it('attaches the OIDC access token as a bearer token', async () => {
    configureOidc(oidcConfig);
    FakeUserManager.latest().user = makeOidcUser({ access_token: 'abc' });
    const seen = serveProtected();

    await apiClient.get('/protected-resource');

    expect(seen).toEqual(['Bearer abc']);
  });

  it('sends no credentials when the server has auth disabled', async () => {
    const seen = serveProtected();

    await apiClient.get('/protected-resource');

    expect(seen).toEqual([null]);
  });

  it('sends no credentials in local mode', async () => {
    configureOidc(oidcConfig);
    FakeUserManager.latest().user = makeOidcUser({ access_token: 'abc' });
    useModeStore.setState({ mode: 'local' });
    const seen = serveProtected();

    await apiClient.get('/protected-resource');

    expect(seen).toEqual([null]);
  });
});

describe('apiClient response interceptor', () => {
  it('renews the token once and retries a request rejected with 401', async () => {
    configureOidc(oidcConfig);
    const manager = FakeUserManager.latest();
    manager.user = makeOidcUser({ access_token: 'stale' });
    manager.signinSilent.mockImplementation(async () => {
      const renewed = makeOidcUser({ access_token: 'fresh' });
      manager.load(renewed);
      return renewed;
    });
    const seen = serveProtected([401]);

    const response = await apiClient.get('/protected-resource');

    expect(response.data).toEqual({ ok: true });
    expect(seen).toEqual(['Bearer stale', 'Bearer fresh']);
    expect(manager.signinSilent).toHaveBeenCalledTimes(1);
  });

  it('signs out and redirects to login when renewal fails', async () => {
    const location = stubLocation();
    const clear = vi.spyOn(queryClient, 'clear');
    configureOidc(oidcConfig);
    const manager = FakeUserManager.latest();
    manager.user = makeOidcUser();
    manager.signinSilent.mockRejectedValue(new Error('invalid_grant'));
    serveProtected([401]);

    await expect(apiClient.get('/protected-resource')).rejects.toBeTruthy();

    expect(manager.removeUser).toHaveBeenCalled();
    expect(clear).toHaveBeenCalled();
    expect(location.href).toBe(
      `/login?returnTo=${encodeURIComponent('/projects/ws-1?tab=jobs')}`,
    );
  });

  it('gives up after one retry if the renewed token is also rejected', async () => {
    const location = stubLocation();
    configureOidc(oidcConfig);
    const manager = FakeUserManager.latest();
    manager.user = makeOidcUser();
    manager.signinSilent.mockResolvedValue(
      makeOidcUser({ access_token: 'fresh' }),
    );
    const seen = serveProtected([401, 401]);

    await expect(apiClient.get('/protected-resource')).rejects.toBeTruthy();

    expect(seen).toHaveLength(2);
    expect(manager.signinSilent).toHaveBeenCalledTimes(1);
    expect(location.href).toMatch(/^\/login/);
  });

  it('does not redirect in local mode', async () => {
    const location = stubLocation();
    const before = location.href;
    useModeStore.setState({ mode: 'local' });
    serveProtected([401]);

    await expect(apiClient.get('/protected-resource')).rejects.toBeTruthy();

    expect(location.href).toBe(before);
  });

  it('does not redirect when the server has auth disabled', async () => {
    const location = stubLocation();
    const before = location.href;
    serveProtected([401]);

    await expect(apiClient.get('/protected-resource')).rejects.toBeTruthy();

    expect(location.href).toBe(before);
  });

  it('passes other errors through untouched', async () => {
    configureOidc(oidcConfig);
    const manager = FakeUserManager.latest();
    manager.user = makeOidcUser();
    serveProtected([403]);

    await expect(apiClient.get('/protected-resource')).rejects.toMatchObject({
      response: { status: 403 },
    });
    expect(manager.signinSilent).not.toHaveBeenCalled();
    expect(manager.removeUser).not.toHaveBeenCalled();
  });
});
