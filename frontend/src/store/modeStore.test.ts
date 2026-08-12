import { HttpResponse, http } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { queryClient } from '@/lib/queryClient';
import { server } from '@/test/handlers';
import { useModeStore } from './modeStore';

beforeEach(() => {
  useModeStore.setState({ mode: null, features: {}, loading: true });
  // Match the networkMode the client is constructed with
  queryClient.setDefaultOptions({
    queries: { refetchOnWindowFocus: false, retry: 1, networkMode: 'always' },
    mutations: { networkMode: 'always' },
  });
});

describe('fetchMode', () => {
  it('sets mode and features from the /version response', async () => {
    server.use(
      http.get('/api/v1/version', () =>
        HttpResponse.json({
          mode: 'local',
          features: { registries: true },
          version: '1.0.0',
        }),
      ),
    );

    await useModeStore.getState().fetchMode();

    const { mode, features, loading } = useModeStore.getState();
    expect(mode).toBe('local');
    expect(features).toEqual({ registries: true });
    expect(loading).toBe(false);
  });

  it('defaults to team mode when the request fails', async () => {
    server.use(http.get('/api/v1/version', () => HttpResponse.error()));

    await useModeStore.getState().fetchMode();

    const { mode, loading } = useModeStore.getState();
    expect(mode).toBe('team');
    expect(loading).toBe(false);
  });

  it('keeps networkMode always in local mode', async () => {
    server.use(
      http.get('/api/v1/version', () =>
        HttpResponse.json({ mode: 'local', version: '1.0.0' }),
      ),
    );

    await useModeStore.getState().fetchMode();

    expect(queryClient.getDefaultOptions().queries?.networkMode).toBe('always');
    expect(queryClient.getDefaultOptions().mutations?.networkMode).toBe(
      'always',
    );
    // Pre-existing defaults are preserved
    expect(queryClient.getDefaultOptions().queries?.retry).toBe(1);
  });

  it('switches to networkMode online in team mode', async () => {
    server.use(
      http.get('/api/v1/version', () =>
        HttpResponse.json({ mode: 'team', version: '1.0.0' }),
      ),
    );

    await useModeStore.getState().fetchMode();

    expect(queryClient.getDefaultOptions().queries?.networkMode).toBe('online');
    expect(queryClient.getDefaultOptions().mutations?.networkMode).toBe(
      'online',
    );
  });

  it('switches to networkMode online when the request fails (team fallback)', async () => {
    server.use(http.get('/api/v1/version', () => HttpResponse.error()));

    await useModeStore.getState().fetchMode();

    expect(queryClient.getDefaultOptions().queries?.networkMode).toBe('online');
  });

  it('defaults features to empty object when not present in response', async () => {
    server.use(
      http.get('/api/v1/version', () =>
        HttpResponse.json({ mode: 'team', version: '1.0.0' }),
      ),
    );

    await useModeStore.getState().fetchMode();

    expect(useModeStore.getState().features).toEqual({});
  });
});

describe('isLocalMode', () => {
  it('returns true when mode is local', () => {
    useModeStore.setState({ mode: 'local', features: {}, loading: false });
    expect(useModeStore.getState().isLocalMode()).toBe(true);
  });

  it('returns false when mode is team', () => {
    useModeStore.setState({ mode: 'team', features: {}, loading: false });
    expect(useModeStore.getState().isLocalMode()).toBe(false);
  });

  it('returns false when mode is null', () => {
    expect(useModeStore.getState().isLocalMode()).toBe(false);
  });
});
