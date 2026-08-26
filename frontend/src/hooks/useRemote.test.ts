import { act, renderHook, waitFor } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';
import {
  mockFederatedIdentity,
  mockFederatedIdentityReview,
  mockJob,
  mockRegistry,
  mockUser,
  mockWorkspace,
  server,
} from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import {
  ERROR_BACKOFF_INTERVAL,
  pollWithErrorBackoff,
  retryWhileUnreachable,
  useApproveRemoteFederatedIdentityReview,
  useConnectServer,
  useCreateRemoteRegistry,
  useCreateRemoteWorkspace,
  useDeleteRemoteRegistry,
  useDeleteRemoteWorkspace,
  useDiscardRemoteFederatedIdentityReview,
  useDisconnectServer,
  useRejectRemoteFederatedIdentityReview,
  useRemoteFederatedIdentityReviews,
  useRemoteJobs,
  useRemoteRegistries,
  useRemoteServer,
  useRemoteUsers,
  useRemoteView,
  useRemoteWorkspace,
  useRemoteWorkspaces,
  useUpdateRemoteRegistry,
} from './useRemote';

const mockRemoteServer = {
  url: 'https://remote.example.com',
  connected: true,
  token: 'remote-token',
};

const mockRemoteWorkspace = {
  ...mockWorkspace,
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

describe('useConnectServer', () => {
  it('calls the connect endpoint and returns server info', async () => {
    server.use(
      http.post('/api/v1/remote/connect', () =>
        HttpResponse.json(mockRemoteServer),
      ),
    );
    const { result } = renderHook(() => useConnectServer(), {
      wrapper: createWrapper(),
    });
    result.current.mutate({
      url: 'https://remote.example.com',
      username: 'user',
      password: 'pass',
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toMatchObject({ connected: true });
  });

  it('enters error state when connect fails', async () => {
    server.use(
      http.post('/api/v1/remote/connect', () =>
        HttpResponse.json({ error: 'unauthorized' }, { status: 401 }),
      ),
    );
    const { result } = renderHook(() => useConnectServer(), {
      wrapper: createWrapper(),
    });
    result.current.mutate({
      url: 'https://bad.example.com',
      username: 'user',
      password: 'pass',
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
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

describe('useRemoteWorkspaces', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/remote/workspaces', () =>
        HttpResponse.json([mockRemoteWorkspace]),
      ),
    );
  });

  it('fetches remote workspaces when enabled', async () => {
    const { result } = renderHook(() => useRemoteWorkspaces(true), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
  });

  it('does not fetch when disabled', () => {
    const { result } = renderHook(() => useRemoteWorkspaces(false), {
      wrapper: createWrapper(),
    });
    expect(result.current.fetchStatus).toBe('idle');
  });

  it('reflects an error state when the remote server is unreachable', async () => {
    server.use(
      http.get('/api/v1/remote/workspaces', () => HttpResponse.error()),
    );
    const { result } = renderHook(() => useRemoteWorkspaces(true), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it('reports isFirstLoad until the query first resolves', async () => {
    const { result } = renderHook(() => useRemoteWorkspaces(true), {
      wrapper: createWrapper(),
    });
    expect(result.current.isFirstLoad).toBe(true);
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.isFirstLoad).toBe(false);
    expect(result.current.isUnreachable).toBe(false);
  });

  it('reports isUnreachable (and clears isFirstLoad) once the query errors', async () => {
    server.use(
      http.get('/api/v1/remote/workspaces', () => HttpResponse.error()),
    );
    const { result } = renderHook(() => useRemoteWorkspaces(true), {
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
      http.get('/api/v1/remote/workspaces', async () => {
        requests += 1;
        if (requests === 1) {
          return HttpResponse.error();
        }
        // Hang the retry long enough for the pending window to be observable.
        await new Promise((resolve) => setTimeout(resolve, 300));
        return HttpResponse.json([mockRemoteWorkspace]);
      }),
    );
    const { result } = renderHook(() => useRemoteWorkspaces(true), {
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
        return useRemoteWorkspaces(true);
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

describe('useRemoteWorkspace', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/remote/workspaces/:id', ({ params }) =>
        HttpResponse.json({ ...mockRemoteWorkspace, id: params.id }),
      ),
    );
  });

  it('fetches a single remote workspace by id', async () => {
    const { result } = renderHook(() => useRemoteWorkspace('ws-1'), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe('ws-1');
  });

  it('does not fetch when id is empty', () => {
    const { result } = renderHook(() => useRemoteWorkspace(''), {
      wrapper: createWrapper(),
    });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useCreateRemoteWorkspace', () => {
  it('calls the create endpoint and returns the new workspace', async () => {
    server.use(
      http.post('/api/v1/remote/workspaces', () =>
        HttpResponse.json(mockRemoteWorkspace, { status: 201 }),
      ),
    );
    const { result } = renderHook(() => useCreateRemoteWorkspace(), {
      wrapper: createWrapper(),
    });
    result.current.mutate({ name: 'New Remote WS' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useDeleteRemoteWorkspace', () => {
  it('calls the delete endpoint successfully', async () => {
    server.use(
      http.delete(
        '/api/v1/remote/workspaces/:id',
        () => new HttpResponse(null, { status: 204 }),
      ),
    );
    const { result } = renderHook(() => useDeleteRemoteWorkspace(), {
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

describe('useRemoteFederatedIdentityReviews', () => {
  it('fetches remote federated identity reviews when enabled', async () => {
    const { result } = renderHook(
      () => useRemoteFederatedIdentityReviews(true),
      {
        wrapper: createWrapper(),
      },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockFederatedIdentityReview]);
  });

  it('does not fetch when disabled', () => {
    const { result } = renderHook(
      () => useRemoteFederatedIdentityReviews(false),
      {
        wrapper: createWrapper(),
      },
    );
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useApproveRemoteFederatedIdentityReview', () => {
  it('calls the remote approve endpoint successfully', async () => {
    server.use(
      http.post(
        '/api/v1/remote/admin/federated-identity-reviews/:id/approve',
        () => HttpResponse.json(mockFederatedIdentity, { status: 201 }),
      ),
    );
    const { result } = renderHook(
      () => useApproveRemoteFederatedIdentityReview(),
      {
        wrapper: createWrapper(),
      },
    );
    result.current.mutate('review-1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(mockFederatedIdentity);
  });
});

describe('useRejectRemoteFederatedIdentityReview', () => {
  it('calls the remote reject endpoint successfully', async () => {
    server.use(
      http.post(
        '/api/v1/remote/admin/federated-identity-reviews/:id/reject',
        () => new HttpResponse(null, { status: 204 }),
      ),
    );
    const { result } = renderHook(
      () => useRejectRemoteFederatedIdentityReview(),
      {
        wrapper: createWrapper(),
      },
    );
    result.current.mutate('review-1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useDiscardRemoteFederatedIdentityReview', () => {
  it('calls the remote discard endpoint successfully', async () => {
    server.use(
      http.delete(
        '/api/v1/remote/admin/federated-identity-reviews/:id',
        () => new HttpResponse(null, { status: 204 }),
      ),
    );
    const { result } = renderHook(
      () => useDiscardRemoteFederatedIdentityReview(),
      {
        wrapper: createWrapper(),
      },
    );
    result.current.mutate('review-1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});
