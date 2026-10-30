import type { User as OidcUser } from 'oidc-client-ts';
import { create } from 'zustand';
import { authApi } from '@/api/auth';
import { configureOidc, resetOidc } from '@/lib/oidc';
import type { AuthConfig, User } from '@/types';

type AuthStatus = 'idle' | 'loading' | 'ready' | 'error';

interface AuthState {
  // How the server authenticates requests; null until initialize resolves.
  config: AuthConfig | null;
  status: AuthStatus;
  // The identity provider session (source of truth for the access token).
  oidcUser: OidcUser | null;
  // The Nebi user from /auth/me.
  user: User | null;
  initialize: (mode: 'local' | 'team') => Promise<void>;
  setUser: (user: User | null) => void;
  clearAuth: () => void;
  isOidc: () => boolean;
  isAuthenticated: () => boolean;
}

export const useAuthStore = create<AuthState>()((set, get) => ({
  config: null,
  status: 'idle',
  oidcUser: null,
  user: null,
  initialize: async (mode) => {
    // StrictMode runs effects twice; only the first call does the work.
    const { status } = get();
    if (status === 'loading' || status === 'ready') return;
    set({ status: 'loading' });

    // Local (desktop) mode never authenticates.
    if (mode === 'local') {
      resetOidc();
      set({ config: { type: 'none' }, status: 'ready' });
      return;
    }

    let config: AuthConfig;
    try {
      config = await authApi.getConfig();
    } catch {
      set({ status: 'error' });
      return;
    }

    if (config?.type === 'none') {
      resetOidc();
      set({ config: { type: 'none' }, status: 'ready' });
      return;
    }
    if (config?.type !== 'oidc') {
      // An auth type this build does not understand: fail closed rather than
      // treating it as "no auth".
      set({ status: 'error' });
      return;
    }

    const manager = configureOidc(config);
    manager.events.addUserLoaded((oidcUser) => set({ oidcUser }));
    manager.events.addUserUnloaded(() => set({ oidcUser: null, user: null }));

    let oidcUser = await manager.getUser().catch(() => null);
    if (oidcUser?.expired) {
      // A reload after the access token lapsed: try the refresh token before
      // deciding the user has to sign in again.
      oidcUser = oidcUser.refresh_token
        ? await manager.signinSilent().catch(() => null)
        : null;
      if (!oidcUser) await manager.removeUser().catch(() => undefined);
    }
    set({ config, oidcUser, status: 'ready' });
  },
  setUser: (user) => set({ user }),
  clearAuth: () => set({ oidcUser: null, user: null }),
  isOidc: () => get().config?.type === 'oidc',
  isAuthenticated: () => {
    const { config, oidcUser } = get();
    if (config?.type === 'none') return true;
    return !!oidcUser && !oidcUser.expired;
  },
}));
