import { renderHook, waitFor } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { describe, expect, it } from 'vitest';
import { mockWorkspace, server } from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import {
  useCreateWorkspace,
  useDeleteWorkspace,
  useInstallWorkspace,
  useUninstallWorkspace,
  useWorkspace,
  useWorkspaces,
} from './useWorkspaces';

describe('useWorkspaces', () => {
  it('fetches and returns the workspace list', async () => {
    const { result } = renderHook(() => useWorkspaces(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockWorkspace]);
  });

  it('reflects an error state when the request fails', async () => {
    server.use(http.get('/api/v1/workspaces', () => HttpResponse.error()));
    const { result } = renderHook(() => useWorkspaces(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useWorkspace', () => {
  it('fetches a single workspace by id', async () => {
    const { result } = renderHook(() => useWorkspace('ws-1'), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe('ws-1');
  });

  it('does not fetch when id is empty', () => {
    const { result } = renderHook(() => useWorkspace(''), {
      wrapper: createWrapper(),
    });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useCreateWorkspace', () => {
  it('calls the create endpoint and returns the new workspace', async () => {
    const { result } = renderHook(() => useCreateWorkspace(), {
      wrapper: createWrapper(),
    });
    result.current.mutate({ name: 'new-workspace' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.name).toBe('new-workspace');
  });

  it('enters error state when the request fails', async () => {
    server.use(
      http.post('/api/v1/workspaces', () =>
        HttpResponse.json({ error: 'conflict' }, { status: 409 }),
      ),
    );
    const { result } = renderHook(() => useCreateWorkspace(), {
      wrapper: createWrapper(),
    });
    result.current.mutate({ name: 'bad' });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useInstallWorkspace', () => {
  it('posts to the install endpoint and returns the queued job', async () => {
    server.use(
      http.post('/api/v1/workspaces/ws-1/install', () =>
        HttpResponse.json(
          {
            id: 'job-1',
            workspace_id: 'ws-1',
            type: 'env_install',
            status: 'pending',
          },
          { status: 202 },
        ),
      ),
    );
    const { result } = renderHook(() => useInstallWorkspace('ws-1'), {
      wrapper: createWrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.type).toBe('env_install');
  });

  it('enters error state when install is rejected', async () => {
    server.use(
      http.post('/api/v1/workspaces/ws-1/install', () =>
        HttpResponse.json({ error: 'already in progress' }, { status: 409 }),
      ),
    );
    const { result } = renderHook(() => useInstallWorkspace('ws-1'), {
      wrapper: createWrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useUninstallWorkspace', () => {
  it('posts to the uninstall endpoint and returns the queued job', async () => {
    server.use(
      http.post('/api/v1/workspaces/ws-1/uninstall', () =>
        HttpResponse.json(
          {
            id: 'job-2',
            workspace_id: 'ws-1',
            type: 'env_uninstall',
            status: 'pending',
          },
          { status: 202 },
        ),
      ),
    );
    const { result } = renderHook(() => useUninstallWorkspace('ws-1'), {
      wrapper: createWrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.type).toBe('env_uninstall');
  });
});

describe('useDeleteWorkspace', () => {
  it('calls the delete endpoint successfully', async () => {
    const { result } = renderHook(() => useDeleteWorkspace(), {
      wrapper: createWrapper(),
    });
    result.current.mutate('ws-1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});
