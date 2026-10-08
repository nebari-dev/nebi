import { HttpResponse, http } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { queryClient } from '@/lib/queryClient';
import { server } from '@/test/handlers';
import { useRuntimeConfigStore } from './runtimeConfigStore';

beforeEach(() => {
  useRuntimeConfigStore.setState({
    features: {},
    logoutUrl: null,
    loading: true,
  });
});

describe('runtime configuration', () => {
  it('loads features and gateway logout URL', async () => {
    server.use(
      http.get('/api/v1/version', () =>
        HttpResponse.json({
          features: { registries: true },
          logout_url: '/logout',
        }),
      ),
    );
    await useRuntimeConfigStore.getState().fetchConfig();
    expect(useRuntimeConfigStore.getState()).toMatchObject({
      features: { registries: true },
      logoutUrl: '/logout',
      loading: false,
    });
  });

  it('recovers when the backend starts on a retry', async () => {
    let calls = 0;
    server.use(
      http.get('/api/v1/version', () => {
        calls++;
        return calls < 2
          ? HttpResponse.error()
          : HttpResponse.json({ version: '1.0.0' });
      }),
    );
    await useRuntimeConfigStore.getState().fetchConfig();
    expect(calls).toBe(2);
    expect(useRuntimeConfigStore.getState().loading).toBe(false);
  });

  it('finishes loading after three failures without changing the app network policy', async () => {
    let calls = 0;
    server.use(
      http.get('/api/v1/version', () => {
        calls++;
        return HttpResponse.error();
      }),
    );
    await useRuntimeConfigStore.getState().fetchConfig();
    expect(calls).toBe(3);
    expect(useRuntimeConfigStore.getState()).toMatchObject({
      features: {},
      logoutUrl: null,
      loading: false,
    });
    expect(queryClient.getDefaultOptions().queries?.networkMode).toBe('always');
    expect(queryClient.getDefaultOptions().mutations?.networkMode).toBe(
      'always',
    );
  });
});
