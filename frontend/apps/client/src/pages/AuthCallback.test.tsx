import { Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { configureOidc, resetOidc } from '@/lib/oidc';
import { CurrentPath } from '@/test/CurrentPath';
import { FakeUserManager, makeOidcUser } from '@/test/fakeOidc';
import { renderWithProviders, screen } from '@/test/utils';
import { AuthCallback } from './AuthCallback';

vi.mock('oidc-client-ts', () => import('@/test/fakeOidc'));

const renderCallback = () =>
  renderWithProviders(
    <Routes>
      <Route path="/auth/callback" element={<AuthCallback />} />
      <Route path="*" element={<CurrentPath />} />
    </Routes>,
    { initialEntries: ['/auth/callback?code=abc&state=xyz'] },
  );

const configure = () => {
  configureOidc({
    type: 'oidc',
    issuer_url: 'https://auth.example.com/realms/nebi',
    client_id: 'nebi',
    scopes: ['openid'],
  });
  return FakeUserManager.latest();
};

describe('AuthCallback', () => {
  beforeEach(() => {
    resetOidc();
    FakeUserManager.reset();
  });

  it('completes the sign-in and returns to the requested page', async () => {
    const manager = configure();
    manager.signinRedirectCallback.mockResolvedValue(
      makeOidcUser({ state: { returnTo: '/projects/ws-1' } }),
    );

    renderCallback();

    expect(await screen.findByText('at /projects/ws-1')).toBeInTheDocument();
    expect(manager.signinRedirectCallback).toHaveBeenCalledTimes(1);
  });

  it('falls back to the home page for a missing or off-site returnTo', async () => {
    const manager = configure();
    manager.signinRedirectCallback.mockResolvedValue(
      makeOidcUser({ state: { returnTo: '//evil.example.com' } }),
    );

    renderCallback();

    expect(await screen.findByText('at /')).toBeInTheDocument();
  });

  it('sends the user back to login when the code exchange fails', async () => {
    const manager = configure();
    manager.signinRedirectCallback.mockRejectedValue(
      new Error('access_denied'),
    );

    renderCallback();

    expect(
      await screen.findByText('at /login?error=login_failed'),
    ).toBeInTheDocument();
  });

  it('leaves the callback when OIDC is not in use', async () => {
    renderCallback();

    expect(await screen.findByText('at /')).toBeInTheDocument();
  });
});
