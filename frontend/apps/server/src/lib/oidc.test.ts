import { beforeEach, describe, expect, it, vi } from 'vitest';
import { FakeUserManager, makeOidcUser } from '@/test/fakeOidc';
import {
  configureOidc,
  getAccessToken,
  renewAccessToken,
  resetOidc,
  safeReturnTo,
} from './oidc';

vi.mock('oidc-client-ts', () => import('@/test/fakeOidc'));

const config = {
  type: 'oidc' as const,
  issuer_url: 'https://auth.example.com/realms/nebi',
  client_id: 'nebi',
  scopes: ['openid'],
};

beforeEach(() => {
  resetOidc();
  FakeUserManager.reset();
});

describe('getAccessToken', () => {
  it('returns null when OIDC is not configured', async () => {
    expect(await getAccessToken()).toBeNull();
  });

  it('returns null when nobody is signed in', async () => {
    configureOidc(config);
    expect(await getAccessToken()).toBeNull();
  });

  it('returns the current access token', async () => {
    configureOidc(config);
    FakeUserManager.latest().user = makeOidcUser({ access_token: 'abc' });
    expect(await getAccessToken()).toBe('abc');
  });

  it('renews an expired access token first', async () => {
    configureOidc(config);
    const manager = FakeUserManager.latest();
    manager.user = makeOidcUser({ expired: true });
    manager.signinSilent.mockResolvedValue(
      makeOidcUser({ access_token: 'fresh' }),
    );

    expect(await getAccessToken()).toBe('fresh');
  });
});

describe('renewAccessToken', () => {
  it('shares one renewal between concurrent callers', async () => {
    configureOidc(config);
    const manager = FakeUserManager.latest();
    manager.user = makeOidcUser();
    manager.signinSilent.mockResolvedValue(
      makeOidcUser({ access_token: 'fresh' }),
    );

    const tokens = await Promise.all([
      renewAccessToken(),
      renewAccessToken(),
      renewAccessToken(),
    ]);

    expect(tokens).toEqual(['fresh', 'fresh', 'fresh']);
    expect(manager.signinSilent).toHaveBeenCalledTimes(1);
  });

  it('does not try to renew without a refresh token', async () => {
    configureOidc(config);
    const manager = FakeUserManager.latest();
    manager.user = makeOidcUser({ refresh_token: undefined });

    expect(await renewAccessToken()).toBeNull();
    expect(manager.signinSilent).not.toHaveBeenCalled();
  });

  it('resolves to null when the identity provider refuses', async () => {
    configureOidc(config);
    const manager = FakeUserManager.latest();
    manager.user = makeOidcUser();
    manager.signinSilent.mockRejectedValue(new Error('invalid_grant'));

    expect(await renewAccessToken()).toBeNull();
  });
});

describe('safeReturnTo', () => {
  it.each(['/projects', '/projects/ws-1?tab=jobs', '/admin/users'])(
    'allows the in-app path %s',
    (path) => {
      expect(safeReturnTo(path)).toBe(path);
    },
  );

  it.each([
    'https://evil.example.com',
    '//evil.example.com',
    '/\\evil.example.com',
    'projects',
    '/login?error=login_failed',
    '/auth/callback?code=x',
    undefined,
    42,
  ])('rejects %s', (value) => {
    expect(safeReturnTo(value)).toBeNull();
  });
});
