import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server, mockUser, mockAdminUser, mockOwnerCollaborator, mockCollaborator } from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import {
  useIsAdmin,
  useUsers,
  useCreateUser,
  useDeleteUser,
  useCollaborators,
  useShareWorkspace,
  useUnshareWorkspace,
  useDashboardStats,
} from './useAdmin';

describe('useIsAdmin', () => {
  it('returns true when admin endpoint succeeds', async () => {
    const { result } = renderHook(() => useIsAdmin(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBe(true);
  });

  it('returns false when admin endpoint returns 403', async () => {
    server.use(
      http.get('/api/v1/admin/users', () =>
        HttpResponse.json({ error: 'Forbidden' }, { status: 403 })
      )
    );
    const { result } = renderHook(() => useIsAdmin(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBe(false);
  });
});

describe('useUsers', () => {
  it('fetches and returns the user list', async () => {
    const { result } = renderHook(() => useUsers(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockUser, mockAdminUser]);
  });
});

describe('useCreateUser', () => {
  it('calls the create user endpoint', async () => {
    server.use(
      http.post('/api/v1/admin/users', () =>
        HttpResponse.json(mockUser, { status: 201 })
      )
    );
    const { result } = renderHook(() => useCreateUser(), { wrapper: createWrapper() });
    result.current.mutate({ username: 'newuser', email: 'new@example.com', password: 'pass' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });

  it('enters error state when creation fails', async () => {
    server.use(
      http.post('/api/v1/admin/users', () =>
        HttpResponse.json({ error: 'conflict' }, { status: 409 })
      )
    );
    const { result } = renderHook(() => useCreateUser(), { wrapper: createWrapper() });
    result.current.mutate({ username: 'dup', email: 'dup@example.com', password: 'pass' });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useDeleteUser', () => {
  it('calls the delete endpoint successfully', async () => {
    server.use(
      http.delete('/api/v1/admin/users/:id', () =>
        new HttpResponse(null, { status: 204 })
      )
    );
    const { result } = renderHook(() => useDeleteUser(), { wrapper: createWrapper() });
    result.current.mutate('user-1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useCollaborators', () => {
  it('fetches collaborators for a workspace', async () => {
    const { result } = renderHook(() => useCollaborators('ws-1'), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockOwnerCollaborator, mockCollaborator]);
  });

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(() => useCollaborators('ws-1', false), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useShareWorkspace', () => {
  it('calls the share endpoint successfully', async () => {
    const { result } = renderHook(() => useShareWorkspace('ws-1'), { wrapper: createWrapper() });
    result.current.mutate({ user_id: 'user-3', role: 'viewer' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useUnshareWorkspace', () => {
  it('calls the unshare endpoint successfully', async () => {
    const { result } = renderHook(() => useUnshareWorkspace('ws-1'), { wrapper: createWrapper() });
    result.current.mutate('user-2');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useDashboardStats', () => {
  it('fetches and returns dashboard stats', async () => {
    const { result } = renderHook(() => useDashboardStats(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toMatchObject({ total_disk_usage_bytes: 0 });
  });
});
