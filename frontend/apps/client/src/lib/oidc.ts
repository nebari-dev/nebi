import { UserManager, WebStorageStateStore } from 'oidc-client-ts';
import type { OidcAuthConfig } from '@/types';
import { getBasePath } from './basePath';

// The SPA is a public OIDC client (authorization code + PKCE) talking
// directly to the identity provider. The resulting access token is sent to
// the Nebi API as a bearer token; Nebi itself never mints tokens.
let userManager: UserManager | null = null;
let renewal: Promise<string | null> | null = null;

const appUrl = (path: string) =>
  `${window.location.origin}${getBasePath()}${path}`;

export const configureOidc = (config: OidcAuthConfig): UserManager => {
  userManager?.stopSilentRenew();
  userManager = new UserManager({
    authority: config.issuer_url,
    client_id: config.client_id,
    redirect_uri: appUrl('/auth/callback'),
    post_logout_redirect_uri: appUrl('/login'),
    response_type: 'code',
    scope: config.scopes.join(' '),
    // Renews with the refresh token shortly before the access token expires.
    // No silent_redirect_uri is configured, so there is no iframe fallback.
    automaticSilentRenew: true,
    userStore: new WebStorageStateStore({ store: window.localStorage }),
  });
  return userManager;
};

// Null until auth config resolves to OIDC (never set in local mode or when
// the team server runs with auth disabled).
export const getUserManager = (): UserManager | null => userManager;

// Test hook: drop the singleton so each test can configure its own.
export const resetOidc = () => {
  userManager?.stopSilentRenew();
  userManager = null;
  renewal = null;
};

// Uses the refresh token to get a fresh access token. Concurrent callers (a
// burst of requests that all hit 401) share one renewal. Resolves to null
// when there is nothing to renew with or the identity provider refuses.
export const renewAccessToken = (): Promise<string | null> => {
  const manager = userManager;
  if (!manager) return Promise.resolve(null);
  if (!renewal) {
    renewal = (async () => {
      try {
        const user = await manager.getUser();
        if (!user?.refresh_token) return null;
        const renewed = await manager.signinSilent();
        return renewed?.access_token ?? null;
      } catch {
        return null;
      } finally {
        renewal = null;
      }
    })();
  }
  return renewal;
};

// The current access token for API requests, renewing it first when it has
// already expired. Null when not signed in (or not using OIDC at all).
export const getAccessToken = async (): Promise<string | null> => {
  if (!userManager) return null;
  const user = await userManager.getUser().catch(() => null);
  if (!user) return null;
  if (user.expired) return renewAccessToken();
  return user.access_token;
};

// Only same-app paths are allowed as post-login destinations, so a crafted
// returnTo cannot bounce the user to another origin.
export const safeReturnTo = (value: unknown): string | null => {
  if (typeof value !== 'string') return null;
  if (!value.startsWith('/') || value.startsWith('//')) return null;
  if (value.startsWith('/\\')) return null;
  if (value.startsWith('/login') || value.startsWith('/auth/callback')) {
    return null;
  }
  return value;
};
