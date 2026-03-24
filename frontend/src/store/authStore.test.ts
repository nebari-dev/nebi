import { describe, it, expect, beforeEach } from 'vitest';
import { useAuthStore } from './authStore';
import { mockUser } from '@/test/handlers';

beforeEach(() => {
  useAuthStore.setState({ token: null, user: null });
  localStorage.clear();
});

describe('setAuth', () => {
  it('stores token and user in state', () => {
    useAuthStore.getState().setAuth('my-token', mockUser);
    const { token, user } = useAuthStore.getState();
    expect(token).toBe('my-token');
    expect(user).toEqual(mockUser);
  });

  it('writes the token to localStorage', () => {
    useAuthStore.getState().setAuth('my-token', mockUser);
    expect(localStorage.getItem('auth_token')).toBe('my-token');
  });
});

describe('clearAuth', () => {
  it('resets token and user to null', () => {
    useAuthStore.getState().setAuth('my-token', mockUser);
    useAuthStore.getState().clearAuth();
    const { token, user } = useAuthStore.getState();
    expect(token).toBeNull();
    expect(user).toBeNull();
  });

  it('removes the token from localStorage', () => {
    localStorage.setItem('auth_token', 'my-token');
    useAuthStore.getState().clearAuth();
    expect(localStorage.getItem('auth_token')).toBeNull();
  });
});

describe('isAuthenticated', () => {
  it('returns false when no token is set', () => {
    expect(useAuthStore.getState().isAuthenticated()).toBe(false);
  });

  it('returns true after setAuth', () => {
    useAuthStore.getState().setAuth('my-token', mockUser);
    expect(useAuthStore.getState().isAuthenticated()).toBe(true);
  });

  it('returns false after clearAuth', () => {
    useAuthStore.getState().setAuth('my-token', mockUser);
    useAuthStore.getState().clearAuth();
    expect(useAuthStore.getState().isAuthenticated()).toBe(false);
  });
});
