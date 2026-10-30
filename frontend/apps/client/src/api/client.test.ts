import { HttpResponse, http } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { server } from '@/test/handlers';
import { apiClient } from './client';

beforeEach(() => {
  localStorage.clear();
  localStorage.setItem('auth_token', 'stale-token');
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('apiClient', () => {
  it.each(['/projects', '/remote/projects'])(
    'does not send a stored browser auth token to %s',
    async (endpoint) => {
      server.use(
        http.get(`/api/v1${endpoint}`, ({ request }) =>
          HttpResponse.json({
            authorization: request.headers.get('authorization'),
          }),
        ),
      );

      const { data } = await apiClient.get(endpoint);

      expect(data.authorization).toBeNull();
    },
  );

  it.each([
    [401, 'unauthorized'],
    [403, 'identity_review_pending'],
    [403, 'identity_review_rejected'],
  ])(
    'does not redirect on %s (%s) from the local backend',
    async (status, error) => {
      const originalHref = window.location.href;
      const location = { ...window.location, href: originalHref };
      vi.spyOn(window, 'location', 'get').mockReturnValue(location as Location);
      server.use(
        http.get('/api/v1/protected-resource', () =>
          HttpResponse.json({ error }, { status }),
        ),
      );

      await expect(apiClient.get('/protected-resource')).rejects.toBeTruthy();

      expect(localStorage.getItem('auth_token')).toBe('stale-token');
      expect(location.href).toBe(originalHref);
    },
  );
});
