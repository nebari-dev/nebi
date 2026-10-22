import { HttpResponse, http } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { queryClient } from '@/lib/queryClient';
import { server } from '@/test/handlers';
import { useModeStore } from './modeStore';

// Snapshot the constructor defaults at module load (before any test mutates
// them) so restoring them can't drift from lib/queryClient.ts.
const { queries: initialQueryDefaults, mutations: initialMutationDefaults } =
  queryClient.getDefaultOptions();

beforeEach(() => {
  useModeStore.setState({ mode: null, features: {}, loading: true });
  queryClient.setDefaultOptions({
    queries: { ...initialQueryDefaults },
    mutations: { ...initialMutationDefaults },
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

  it('falls back to local mode and keeps networkMode always when the request fails in the desktop app', async () => {
    server.use(http.get('/api/v1/version', () => HttpResponse.error()));
    // The Wails runtime marks the desktop app, which is always local mode: a
    // team fallback would strand it on the Login page (issue #530), and its
    // loopback backend must never be paused by OS offline events (issue #217).
    window.runtime = { BrowserOpenURL: () => {} };

    try {
      await useModeStore.getState().fetchMode();

      expect(useModeStore.getState().mode).toBe('local');
      expect(useModeStore.getState().isLocalMode()).toBe(true);
      expect(queryClient.getDefaultOptions().queries?.networkMode).toBe(
        'always',
      );
    } finally {
      delete window.runtime;
    }
  });

  it('recovers when /version succeeds on a retry', async () => {
    let calls = 0;
    server.use(
      http.get('/api/v1/version', () => {
        calls++;
        if (calls < 2) return HttpResponse.error();
        return HttpResponse.json({ mode: 'local', version: '1.0.0' });
      }),
    );

    await useModeStore.getState().fetchMode();

    expect(useModeStore.getState().mode).toBe('local');
    expect(queryClient.getDefaultOptions().queries?.networkMode).toBe('always');
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
