import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server, mockWorkspace } from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import {
  useWorkspaces,
  useWorkspace,
  useCreateWorkspace,
  useDeleteWorkspace,
} from './useWorkspaces';

describe('useWorkspaces', () => {
  it('fetches and returns the workspace list', async () => {
    const { result } = renderHook(() => useWorkspaces(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockWorkspace]);
  });

  it('reflects an error state when the request fails', async () => {
    server.use(
      http.get('/api/v1/workspaces', () => HttpResponse.error())
    );
    const { result } = renderHook(() => useWorkspaces(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useWorkspace', () => {
  it('fetches a single workspace by id', async () => {
    const { result } = renderHook(() => useWorkspace('ws-1'), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe('ws-1');
  });

  it('does not fetch when id is empty', () => {
    const { result } = renderHook(() => useWorkspace(''), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useCreateWorkspace', () => {
  it('calls the create endpoint and returns the new workspace', async () => {
    const { result } = renderHook(() => useCreateWorkspace(), { wrapper: createWrapper() });
    result.current.mutate({ name: 'new-workspace' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.name).toBe('new-workspace');
  });

  it('enters error state when the request fails', async () => {
    server.use(
      http.post('/api/v1/workspaces', () =>
        HttpResponse.json({ error: 'conflict' }, { status: 409 })
      )
    );
    const { result } = renderHook(() => useCreateWorkspace(), { wrapper: createWrapper() });
    result.current.mutate({ name: 'bad' });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useDeleteWorkspace', () => {
  it('calls the delete endpoint successfully', async () => {
    const { result } = renderHook(() => useDeleteWorkspace(), { wrapper: createWrapper() });
    result.current.mutate('ws-1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});
