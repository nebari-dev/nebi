import { describe, it, expect, beforeEach } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/handlers';
import { useModeStore } from './modeStore';

beforeEach(() => {
  useModeStore.setState({ mode: null, features: {}, loading: true });
});

describe('fetchMode', () => {
  it('sets mode and features from the /version response', async () => {
    server.use(
      http.get('/api/v1/version', () =>
        HttpResponse.json({ mode: 'local', features: { registries: true }, version: '1.0.0' })
      )
    );

    await useModeStore.getState().fetchMode();

    const { mode, features, loading } = useModeStore.getState();
    expect(mode).toBe('local');
    expect(features).toEqual({ registries: true });
    expect(loading).toBe(false);
  });

  it('defaults to team mode when the request fails', async () => {
    server.use(
      http.get('/api/v1/version', () => HttpResponse.error())
    );

    await useModeStore.getState().fetchMode();

    const { mode, loading } = useModeStore.getState();
    expect(mode).toBe('team');
    expect(loading).toBe(false);
  });

  it('defaults features to empty object when not present in response', async () => {
    server.use(
      http.get('/api/v1/version', () =>
        HttpResponse.json({ mode: 'team', version: '1.0.0' })
      )
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
