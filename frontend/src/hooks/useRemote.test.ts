import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server, mockWorkspace, mockJob, mockRegistry, mockUser } from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import {
  useRemoteServer,
  useConnectServer,
  useDisconnectServer,
  useRemoteWorkspaces,
  useRemoteWorkspace,
  useCreateRemoteWorkspace,
  useDeleteRemoteWorkspace,
  useRemoteJobs,
  useRemoteRegistries,
  useRemoteUsers,
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

describe('useRemoteServer', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/remote/server', () => HttpResponse.json(mockRemoteServer))
    );
  });

  it('fetches remote server info', async () => {
    const { result } = renderHook(() => useRemoteServer(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toMatchObject({ connected: true });
  });

  it('reflects an error state when request fails', async () => {
    server.use(
      http.get('/api/v1/remote/server', () => HttpResponse.error())
    );
    const { result } = renderHook(() => useRemoteServer(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useConnectServer', () => {
  it('calls the connect endpoint and returns server info', async () => {
    server.use(
      http.post('/api/v1/remote/connect', () => HttpResponse.json(mockRemoteServer))
    );
    const { result } = renderHook(() => useConnectServer(), { wrapper: createWrapper() });
    result.current.mutate({ url: 'https://remote.example.com', token: 'tok' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toMatchObject({ connected: true });
  });

  it('enters error state when connect fails', async () => {
    server.use(
      http.post('/api/v1/remote/connect', () =>
        HttpResponse.json({ error: 'unauthorized' }, { status: 401 })
      )
    );
    const { result } = renderHook(() => useConnectServer(), { wrapper: createWrapper() });
    result.current.mutate({ url: 'https://bad.example.com', token: 'bad' });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useDisconnectServer', () => {
  it('calls the disconnect endpoint successfully', async () => {
    server.use(
      http.delete('/api/v1/remote/server', () => new HttpResponse(null, { status: 204 }))
    );
    const { result } = renderHook(() => useDisconnectServer(), { wrapper: createWrapper() });
    result.current.mutate();
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useRemoteWorkspaces', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/remote/workspaces', () => HttpResponse.json([mockRemoteWorkspace]))
    );
  });

  it('fetches remote workspaces when enabled', async () => {
    const { result } = renderHook(() => useRemoteWorkspaces(true), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
  });

  it('does not fetch when disabled', () => {
    const { result } = renderHook(() => useRemoteWorkspaces(false), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useRemoteWorkspace', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/remote/workspaces/:id', ({ params }) =>
        HttpResponse.json({ ...mockRemoteWorkspace, id: params.id })
      )
    );
  });

  it('fetches a single remote workspace by id', async () => {
    const { result } = renderHook(() => useRemoteWorkspace('ws-1'), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe('ws-1');
  });

  it('does not fetch when id is empty', () => {
    const { result } = renderHook(() => useRemoteWorkspace(''), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useCreateRemoteWorkspace', () => {
  it('calls the create endpoint and returns the new workspace', async () => {
    server.use(
      http.post('/api/v1/remote/workspaces', () =>
        HttpResponse.json(mockRemoteWorkspace, { status: 201 })
      )
    );
    const { result } = renderHook(() => useCreateRemoteWorkspace(), { wrapper: createWrapper() });
    result.current.mutate({ workspace_id: 'ws-1' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useDeleteRemoteWorkspace', () => {
  it('calls the delete endpoint successfully', async () => {
    server.use(
      http.delete('/api/v1/remote/workspaces/:id', () => new HttpResponse(null, { status: 204 }))
    );
    const { result } = renderHook(() => useDeleteRemoteWorkspace(), { wrapper: createWrapper() });
    result.current.mutate('ws-1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useRemoteJobs', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/remote/jobs', () => HttpResponse.json([mockJob]))
    );
  });

  it('fetches remote jobs when enabled', async () => {
    const { result } = renderHook(() => useRemoteJobs(true), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockJob]);
  });

  it('does not fetch when disabled', () => {
    const { result } = renderHook(() => useRemoteJobs(false), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useRemoteRegistries', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/remote/registries', () => HttpResponse.json([mockRegistry]))
    );
  });

  it('fetches remote registries when enabled', async () => {
    const { result } = renderHook(() => useRemoteRegistries(true), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockRegistry]);
  });

  it('does not fetch when disabled', () => {
    const { result } = renderHook(() => useRemoteRegistries(false), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useRemoteUsers', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/remote/admin/users', () => HttpResponse.json([mockUser]))
    );
  });

  it('fetches remote users when enabled', async () => {
    const { result } = renderHook(() => useRemoteUsers(true), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockUser]);
  });

  it('does not fetch when disabled', () => {
    const { result } = renderHook(() => useRemoteUsers(false), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });
});
