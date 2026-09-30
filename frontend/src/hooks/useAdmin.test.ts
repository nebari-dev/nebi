import { renderHook, waitFor } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { describe, expect, it } from 'vitest';
import {
  mockAdminUser,
  mockCollaborator,
  mockOwnerCollaborator,
  mockUser,
  server,
} from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import {
  useCollaborators,
  useDashboardStats,
  useIsAdmin,
  useShareProject,
  useUnshareProject,
  useUsers,
} from './useAdmin';

describe('useIsAdmin', () => {
  it('returns true when admin endpoint succeeds', async () => {
    const { result } = renderHook(() => useIsAdmin(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBe(true);
  });

  it('returns false when admin endpoint returns 403', async () => {
    server.use(
      http.get('/api/v1/admin/users', () =>
        HttpResponse.json({ error: 'Forbidden' }, { status: 403 }),
      ),
    );
    const { result } = renderHook(() => useIsAdmin(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBe(false);
  });
});

describe('useUsers', () => {
  it('fetches and returns the user list', async () => {
    const { result } = renderHook(() => useUsers(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockUser, mockAdminUser]);
  });
});

describe('useCollaborators', () => {
  it('fetches collaborators for a project', async () => {
    const { result } = renderHook(() => useCollaborators('ws-1'), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([
      mockOwnerCollaborator,
      mockCollaborator,
    ]);
  });

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(() => useCollaborators('ws-1', false), {
      wrapper: createWrapper(),
    });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useShareProject', () => {
  it('calls the share endpoint successfully', async () => {
    const { result } = renderHook(() => useShareProject('ws-1'), {
      wrapper: createWrapper(),
    });
    result.current.mutate({ user_id: 'user-3', role: 'viewer' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useUnshareProject', () => {
  it('calls the unshare endpoint successfully', async () => {
    const { result } = renderHook(() => useUnshareProject('ws-1'), {
      wrapper: createWrapper(),
    });
    result.current.mutate('user-2');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useDashboardStats', () => {
  it('fetches and returns dashboard stats', async () => {
    const { result } = renderHook(() => useDashboardStats(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toMatchObject({ total_disk_usage_bytes: 0 });
  });
});
