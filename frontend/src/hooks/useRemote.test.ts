import { renderHook, waitFor } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
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
  useApproveRemoteFederatedIdentityReview,
  useConnectServer,
  useCreateRemoteWorkspace,
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
    server.use(
      http.get('/api/v1/remote/server', () =>
        HttpResponse.json(mockRemoteServer),
      ),
    );
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
