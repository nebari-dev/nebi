import { act, renderHook, waitFor } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';
import {
  mockJob,
  mockProject,
  mockRegistry,
  mockUser,
  server,
} from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import {
  ERROR_BACKOFF_INTERVAL,
  pollWithErrorBackoff,
  retryWhileUnreachable,
  useCreateRemoteProject,
  useCreateRemoteRegistry,
  useDeleteRemoteProject,
  useDeleteRemoteRegistry,
  useDisconnectServer,
  useRemoteConnect,
  useRemoteJobs,
  useRemoteProject,
  useRemoteProjects,
  useRemoteRegistries,
  useRemoteServer,
  useRemoteUsers,
  useRemoteView,
  useUpdateRemoteRegistry,
} from './useRemote';

const mockRemoteServer = {
  url: 'https://remote.example.com',
  connected: true,
  token: 'remote-token',
};

const mockRemoteProject = {
  ...mockProject,
  server_url: 'https://remote.example.com',
};

describe('pollWithErrorBackoff', () => {
  it('returns the interval while the query is healthy', () => {
    expect(pollWithErrorBackoff(5000)({ state: { status: 'success' } })).toBe(
      5000,
    );
    expect(pollWithErrorBackoff(5000)({ state: { status: 'pending' } })).toBe(
      5000,
    );
  });

  it('backs off to the slow interval once the query errors', () => {
    expect(pollWithErrorBackoff(5000)({ state: { status: 'error' } })).toBe(
      ERROR_BACKOFF_INTERVAL,
    );
  });
});

describe('retryWhileUnreachable', () => {
  it('does not poll while the query is healthy', () => {
    expect(retryWhileUnreachable({ state: { status: 'success' } })).toBe(false);
    expect(retryWhileUnreachable({ state: { status: 'pending' } })).toBe(false);
  });

  it('retries on the error-backoff cadence once the query errors, so the unreachable banner self-heals', () => {
    expect(retryWhileUnreachable({ state: { status: 'error' } })).toBe(
      ERROR_BACKOFF_INTERVAL,
    );
  });
});

describe('useRemoteView', () => {
  it('reports the remote view when local mode, connected, and viewMode is remote', async () => {
    useModeStore.setState({ mode: 'local', features: {}, loading: false });
    useViewModeStore.setState({ viewMode: 'remote' });
    server.use(
      http.get('/api/v1/remote/server', () =>
        HttpResponse.json({
          url: 'https://remote.example.com',
          username: 'user',
          status: 'connected',
        }),
      ),
    );

    const { result } = renderHook(() => useRemoteView(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isRemoteConnected).toBe(true));
    expect(result.current.isRemoteView).toBe(true);
  });

  it('is not connected when no remote server is configured', async () => {
    useModeStore.setState({ mode: 'local', features: {}, loading: false });
    useViewModeStore.setState({ viewMode: 'remote' });
    server.use(
      http.get('/api/v1/remote/server', () =>
        HttpResponse.json({ url: '', username: '', status: 'disconnected' }),
      ),
    );

    const { result } = renderHook(() => useRemoteView(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isRemoteConnected).toBe(false));
    expect(result.current.isRemoteView).toBe(false);
  });

  it('is not the remote view when viewMode is local', async () => {
    useModeStore.setState({ mode: 'local', features: {}, loading: false });
    useViewModeStore.setState({ viewMode: 'local' });
    server.use(
      http.get('/api/v1/remote/server', () =>
        HttpResponse.json({
          url: 'https://remote.example.com',
          username: 'user',
          status: 'connected',
        }),
      ),
    );

    const { result } = renderHook(() => useRemoteView(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isRemoteConnected).toBe(true));
    expect(result.current.isRemoteView).toBe(false);
  });
});

describe('useRemoteServer', () => {
  beforeEach(() => {
    useModeStore.setState({ mode: 'local', features: {}, loading: false });
    server.use(
      http.get('/api/v1/remote/server', () =>
        HttpResponse.json(mockRemoteServer),
      ),
    );
  });

  afterEach(() => {
    useModeStore.setState({ mode: null, features: {}, loading: true });
  });

  it('does not fetch in team mode', async () => {
    useModeStore.setState({ mode: 'team', features: {}, loading: false });
    const { result } = renderHook(() => useRemoteServer(), {
      wrapper: createWrapper(),
    });
    expect(result.current.fetchStatus).toBe('idle');
    expect(result.current.isPending).toBe(true);
  });

  it('fetches remote server info', async () => {
    const { result } = renderHook(() => useRemoteServer(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toMatchObject({ connected: true });
  });

  it('reflects an error state when request fails', async () => {
    server.use(http.get('/api/v1/remote/server', () => HttpResponse.error()));
    const { result } = renderHook(() => useRemoteServer(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

const mockDeviceAuthorization = {
  user_code: 'ABCD-EFGH',
  verification_uri: 'https://auth.example.com/device',
  verification_uri_complete:
    'https://auth.example.com/device?user_code=ABCD-EFGH',
  expires_in: 600,
  interval: 5,
};

describe('useRemoteConnect', () => {
  let pollCalls: number;
  let pollResponses: Array<() => Response>;

  const usePollHandlers = () => {
    pollCalls = 0;
    server.use(
      http.post('/api/v1/remote/connect', () =>
        HttpResponse.json(mockDeviceAuthorization),
      ),
      http.post('/api/v1/remote/connect/poll', () => {
        pollCalls += 1;
        const next = pollResponses.shift();
        return next ? next() : HttpResponse.json({ status: 'pending' });
      }),
    );
  };

  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    pollResponses = [];
    usePollHandlers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  const advance = (ms: number) =>
    act(async () => {
      await vi.advanceTimersByTimeAsync(ms);
    });

  it('shows the device code and polls until connected', async () => {
    const onConnected = vi.fn();
    pollResponses = [
      () => HttpResponse.json({ status: 'pending' }),
      () =>
        HttpResponse.json({
          status: 'connected',
          url: 'https://remote.example.com',
          username: 'alice',
        }),
    ];
    const { result } = renderHook(() => useRemoteConnect({ onConnected }), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      await result.current.start('https://remote.example.com');
    });
    expect(result.current.state).toEqual({
      status: 'pending',
      authorization: mockDeviceAuthorization,
    });
    expect(pollCalls).toBe(0);

    await advance(5000);
    await waitFor(() => expect(pollCalls).toBe(1));
    expect(result.current.state.status).toBe('pending');

    await advance(5000);
    await waitFor(() => expect(result.current.state.status).toBe('idle'));
    expect(pollCalls).toBe(2);
    expect(onConnected).toHaveBeenCalledWith({
      status: 'connected',
      url: 'https://remote.example.com',
      username: 'alice',
    });

    // Polling stops once connected.
    await advance(20000);
    expect(pollCalls).toBe(2);
  });

  it('connects without polling when the remote has auth disabled', async () => {
    const onConnected = vi.fn();
    const connected = {
      status: 'connected',
      url: 'https://remote.example.com',
      username: 'admin',
    };
    server.use(
      http.post('/api/v1/remote/connect', () => HttpResponse.json(connected)),
    );
    const { result } = renderHook(() => useRemoteConnect({ onConnected }), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      await result.current.start('https://remote.example.com');
    });

    expect(result.current.state).toEqual({ status: 'idle' });
    expect(onConnected).toHaveBeenCalledWith(connected);
    await advance(20000);
    expect(pollCalls).toBe(0);
  });

  it('slows down when the backend asks for a longer interval', async () => {
    pollResponses = [
      () => HttpResponse.json({ status: 'pending', interval: 10 }),
    ];
    const { result } = renderHook(() => useRemoteConnect(), {
      wrapper: createWrapper(),
    });
    await act(async () => {
      await result.current.start('https://remote.example.com');
    });

    await advance(5000);
    await waitFor(() => expect(pollCalls).toBe(1));

    await advance(5000);
    expect(pollCalls).toBe(1);

    await advance(5000);
    await waitFor(() => expect(pollCalls).toBe(2));
  });

  it('stops with the backend error when the flow fails', async () => {
    pollResponses = [
      () => HttpResponse.json({ error: 'access denied' }, { status: 400 }),
    ];
    const { result } = renderHook(() => useRemoteConnect(), {
      wrapper: createWrapper(),
    });
    await act(async () => {
      await result.current.start('https://remote.example.com');
    });

    await advance(5000);
    await waitFor(() =>
      expect(result.current.state).toEqual({
        status: 'error',
        error: 'access denied',
      }),
    );

    await advance(20000);
    expect(pollCalls).toBe(1);
  });

  it('keeps polling through transient server errors', async () => {
    pollResponses = [
      () => HttpResponse.json({ error: 'bad gateway' }, { status: 502 }),
    ];
    const { result } = renderHook(() => useRemoteConnect(), {
      wrapper: createWrapper(),
    });
    await act(async () => {
      await result.current.start('https://remote.example.com');
    });

    await advance(5000);
    await waitFor(() => expect(pollCalls).toBe(1));
    expect(result.current.state.status).toBe('pending');

    await advance(5000);
    await waitFor(() => expect(pollCalls).toBe(2));
  });

  it('reports a failure to start the flow', async () => {
    server.use(
      http.post('/api/v1/remote/connect', () =>
        HttpResponse.json(
          { error: 'remote server has authentication disabled' },
          { status: 400 },
        ),
      ),
    );
    const { result } = renderHook(() => useRemoteConnect(), {
      wrapper: createWrapper(),
    });
    await act(async () => {
      await result.current.start('https://remote.example.com');
    });

    expect(result.current.state).toEqual({
      status: 'error',
      error: 'remote server has authentication disabled',
    });
  });

  it('stops polling when cancelled', async () => {
    const { result } = renderHook(() => useRemoteConnect(), {
      wrapper: createWrapper(),
    });
    await act(async () => {
      await result.current.start('https://remote.example.com');
    });

    act(() => result.current.cancel());
    expect(result.current.state).toEqual({ status: 'idle' });

    await advance(20000);
    expect(pollCalls).toBe(0);
  });

  it('stops polling on unmount', async () => {
    const { result, unmount } = renderHook(() => useRemoteConnect(), {
      wrapper: createWrapper(),
    });
    await act(async () => {
      await result.current.start('https://remote.example.com');
    });

    unmount();
    await advance(20000);
    expect(pollCalls).toBe(0);
  });
});

describe('useDisconnectServer', () => {
  it('calls the disconnect endpoint successfully', async () => {
    server.use(
      http.delete(
        '/api/v1/remote/server',
        () => new HttpResponse(null, { status: 204 }),
      ),
    );
    const { result } = renderHook(() => useDisconnectServer(), {
      wrapper: createWrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useRemoteProjects', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/remote/projects', () =>
        HttpResponse.json([mockRemoteProject]),
      ),
    );
  });

  it('fetches remote projects when enabled', async () => {
    const { result } = renderHook(() => useRemoteProjects(true), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
  });

  it('does not fetch when disabled', () => {
    const { result } = renderHook(() => useRemoteProjects(false), {
      wrapper: createWrapper(),
    });
    expect(result.current.fetchStatus).toBe('idle');
  });

  it('reflects an error state when the remote server is unreachable', async () => {
    server.use(http.get('/api/v1/remote/projects', () => HttpResponse.error()));
    const { result } = renderHook(() => useRemoteProjects(true), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it('reports isFirstLoad until the query first resolves', async () => {
    const { result } = renderHook(() => useRemoteProjects(true), {
      wrapper: createWrapper(),
    });
    expect(result.current.isFirstLoad).toBe(true);
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.isFirstLoad).toBe(false);
    expect(result.current.isUnreachable).toBe(false);
  });

  it('reports isUnreachable (and clears isFirstLoad) once the query errors', async () => {
    server.use(http.get('/api/v1/remote/projects', () => HttpResponse.error()));
    const { result } = renderHook(() => useRemoteProjects(true), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isUnreachable).toBe(true));
    expect(result.current.isFirstLoad).toBe(false);
  });

  it('holds isUnreachable while a retry refetch is in flight, then clears it on success', async () => {
    // Refetching a never-succeeded query resets it to pending and clears
    // isError, so isUnreachable must bridge that window or the banner
    // flashes off during every failed retry.
    let requests = 0;
    server.use(
      http.get('/api/v1/remote/projects', async () => {
        requests += 1;
        if (requests === 1) {
          return HttpResponse.error();
        }
        // Hang the retry long enough for the pending window to be observable.
        await new Promise((resolve) => setTimeout(resolve, 300));
        return HttpResponse.json([mockRemoteProject]);
      }),
    );
    const { result } = renderHook(() => useRemoteProjects(true), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isUnreachable).toBe(true));

    result.current.refetch();
    await waitFor(() => expect(result.current.isPending).toBe(true));
    expect(result.current.isUnreachable).toBe(true);
    expect(result.current.isFirstLoad).toBe(false);

    await waitFor(() => expect(result.current.isSuccess).toBe(true), {
      timeout: 2000,
    });
    expect(result.current.isUnreachable).toBe(false);
  });

  it('does not re-render consumers when a refetch returns unchanged data', async () => {
    // withRemoteFlags spreads the query result, which reads every field of
    // TanStack's tracked-props proxy and so marks them all tracked — hence the
    // pinned notifyOnChangeProps on the wrapped queries. Without it, isFetching
    // and dataUpdatedAt re-render every consumer on each 5s poll tick even when
    // the payload is identical.
    let renders = 0;
    const { result } = renderHook(
      () => {
        renders += 1;
        return useRemoteProjects(true);
      },
      { wrapper: createWrapper() },
    );
    await waitFor(() => expect(result.current.data).toBeDefined());

    const rendersAfterLoad = renders;
    // Fire outside act() so the fetch-start and fetch-settle notifications
    // land in separate flushes, the way a real poll tick does — awaiting
    // refetch() inside act() batches them into one and hides the churn.
    const refetched = result.current.refetch();
    await act(async () => {
      await refetched;
      await new Promise((resolve) => setTimeout(resolve, 50));
    });
    expect(result.current.data).toBeDefined();
    expect(renders).toBe(rendersAfterLoad);
  });
});

describe('useRemoteProject', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/remote/projects/:id', ({ params }) =>
        HttpResponse.json({ ...mockRemoteProject, id: params.id }),
      ),
    );
  });

  it('fetches a single remote project by id', async () => {
    const { result } = renderHook(() => useRemoteProject('ws-1'), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe('ws-1');
  });

  it('does not fetch when id is empty', () => {
    const { result } = renderHook(() => useRemoteProject(''), {
      wrapper: createWrapper(),
    });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useCreateRemoteProject', () => {
  it('calls the create endpoint and returns the new project', async () => {
    server.use(
      http.post('/api/v1/remote/projects', () =>
        HttpResponse.json(mockRemoteProject, { status: 201 }),
      ),
    );
    const { result } = renderHook(() => useCreateRemoteProject(), {
      wrapper: createWrapper(),
    });
    result.current.mutate({ name: 'New Remote WS' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useDeleteRemoteProject', () => {
  it('calls the delete endpoint successfully', async () => {
    server.use(
      http.delete(
        '/api/v1/remote/projects/:id',
        () => new HttpResponse(null, { status: 204 }),
      ),
    );
    const { result } = renderHook(() => useDeleteRemoteProject(), {
      wrapper: createWrapper(),
    });
    result.current.mutate('ws-1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useRemoteJobs', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/remote/jobs', () => HttpResponse.json([mockJob])),
    );
  });

  it('fetches remote jobs when enabled', async () => {
    const { result } = renderHook(() => useRemoteJobs(true), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockJob]);
  });

  it('does not fetch when disabled', () => {
    const { result } = renderHook(() => useRemoteJobs(false), {
      wrapper: createWrapper(),
    });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useRemoteRegistries', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/remote/registries', () =>
        HttpResponse.json([mockRegistry]),
      ),
    );
  });

  it('fetches remote registries when enabled', async () => {
    const { result } = renderHook(() => useRemoteRegistries(true), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockRegistry]);
  });

  it('does not fetch when disabled', () => {
    const { result } = renderHook(() => useRemoteRegistries(false), {
      wrapper: createWrapper(),
    });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useCreateRemoteRegistry', () => {
  it('posts to the remote admin registries endpoint, not the local one', async () => {
    let hitRemote = false;
    server.use(
      http.post('/api/v1/remote/admin/registries', () => {
        hitRemote = true;
        return HttpResponse.json(mockRegistry, { status: 201 });
      }),
      http.post('/api/v1/admin/registries', () =>
        HttpResponse.json({ error: 'should not be called' }, { status: 500 }),
      ),
    );
    const { result } = renderHook(() => useCreateRemoteRegistry(), {
      wrapper: createWrapper(),
    });
    result.current.mutate({ name: 'GHCR', url: 'ghcr.io' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(hitRemote).toBe(true);
  });
});

describe('useUpdateRemoteRegistry', () => {
  it('puts to the remote admin registry endpoint, not the local one', async () => {
    let hitRemote = false;
    server.use(
      http.put('/api/v1/remote/admin/registries/reg-1', () => {
        hitRemote = true;
        return HttpResponse.json(mockRegistry, { status: 200 });
      }),
      http.put('/api/v1/admin/registries/reg-1', () =>
        HttpResponse.json({ error: 'should not be called' }, { status: 500 }),
      ),
    );
    const { result } = renderHook(() => useUpdateRemoteRegistry(), {
      wrapper: createWrapper(),
    });
    result.current.mutate({ id: 'reg-1', data: { name: 'GHCR2' } });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(hitRemote).toBe(true);
  });
});

describe('useDeleteRemoteRegistry', () => {
  it('deletes on the remote admin registry endpoint, not the local one', async () => {
    let hitRemote = false;
    server.use(
      http.delete('/api/v1/remote/admin/registries/reg-1', () => {
        hitRemote = true;
        return new HttpResponse(null, { status: 204 });
      }),
      http.delete('/api/v1/admin/registries/reg-1', () =>
        HttpResponse.json({ error: 'should not be called' }, { status: 500 }),
      ),
    );
    const { result } = renderHook(() => useDeleteRemoteRegistry(), {
      wrapper: createWrapper(),
    });
    result.current.mutate('reg-1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(hitRemote).toBe(true);
  });
});

describe('useRemoteUsers', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/remote/admin/users', () =>
        HttpResponse.json([mockUser]),
      ),
    );
  });

  it('fetches remote users when enabled', async () => {
    const { result } = renderHook(() => useRemoteUsers(true), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockUser]);
  });

  it('does not fetch when disabled', () => {
    const { result } = renderHook(() => useRemoteUsers(false), {
      wrapper: createWrapper(),
    });
    expect(result.current.fetchStatus).toBe('idle');
  });
});
