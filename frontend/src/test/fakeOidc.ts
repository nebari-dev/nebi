// A stand-in for oidc-client-ts so tests never talk to an identity provider.
// Use it with:
//   vi.mock('oidc-client-ts', () => import('@/test/fakeOidc'));
import type { User as OidcUser, UserManagerSettings } from 'oidc-client-ts';
import { vi } from 'vitest';

type Listener = (...args: unknown[]) => void;

export type FakeOidcUser = Pick<
  OidcUser,
  'access_token' | 'refresh_token' | 'expired' | 'state' | 'profile'
>;

export const makeOidcUser = (overrides: Partial<FakeOidcUser> = {}): OidcUser =>
  ({
    access_token: 'access-token',
    refresh_token: 'refresh-token',
    expired: false,
    state: undefined,
    profile: { sub: 'user-1', iss: 'https://auth.example.com', aud: 'nebi' },
    ...overrides,
  }) as unknown as OidcUser;

export class FakeUserManager {
  static instances: FakeUserManager[] = [];
  // Runs for each new instance, e.g. to seed a session "stored" by an
  // earlier page load or stub its methods before the app uses it.
  static onCreate: ((manager: FakeUserManager) => void) | null = null;

  settings: UserManagerSettings;
  user: OidcUser | null = null;
  private loaded = new Set<Listener>();
  private unloaded = new Set<Listener>();

  events = {
    addUserLoaded: (cb: Listener) => this.loaded.add(cb),
    addUserUnloaded: (cb: Listener) => this.unloaded.add(cb),
  };

  constructor(settings: UserManagerSettings) {
    this.settings = settings;
    FakeUserManager.instances.push(this);
    FakeUserManager.onCreate?.(this);
  }

  static latest(): FakeUserManager {
    const manager = FakeUserManager.instances.at(-1);
    if (!manager) throw new Error('No UserManager was created');
    return manager;
  }

  static reset() {
    FakeUserManager.instances = [];
    FakeUserManager.onCreate = null;
  }

  // Stores a user the way a successful sign-in or renewal would.
  load(user: OidcUser) {
    this.user = user;
    for (const cb of this.loaded) cb(user);
  }

  getUser = vi.fn(async () => this.user);
  removeUser = vi.fn(async () => {
    this.user = null;
    for (const cb of this.unloaded) cb();
  });
  signinRedirect = vi.fn(async (_args?: unknown) => {});
  signinRedirectCallback = vi.fn(async (): Promise<OidcUser> => {
    throw new Error('signinRedirectCallback not stubbed');
  });
  signinSilent = vi.fn(async (): Promise<OidcUser | null> => {
    throw new Error('signinSilent not stubbed');
  });
  signoutRedirect = vi.fn(async () => {});
  stopSilentRenew = vi.fn();
}

export const UserManager = FakeUserManager;

export class WebStorageStateStore {
  args: unknown;
  constructor(args: unknown) {
    this.args = args;
  }
}
