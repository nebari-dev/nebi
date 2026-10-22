import { HttpResponse, http } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useModeStore } from '@/store/modeStore';
import { server } from '@/test/handlers';
import { apiClient } from './client';

beforeEach(() => {
  localStorage.clear();
  localStorage.setItem('auth_token', 'stale-token');
  useModeStore.setState({ mode: 'team', features: {}, loading: false });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('apiClient response interceptor', () => {
  it('redirects known federated identity review 403 errors to login', async () => {
    const location = { ...window.location, href: window.location.href };
    vi.spyOn(window, 'location', 'get').mockReturnValue(location as Location);
    server.use(
      http.get('/api/v1/protected-resource', () =>
        HttpResponse.json(
          { error: 'identity_review_pending' },
          { status: 403 },
        ),
      ),
    );

    await expect(apiClient.get('/protected-resource')).rejects.toBeTruthy();

    expect(localStorage.getItem('auth_token')).toBeNull();
    expect(location.href).toBe('/login?error=identity_review_pending');
  });
});
