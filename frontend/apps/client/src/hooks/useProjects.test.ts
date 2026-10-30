import { renderHook, waitFor } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { describe, expect, it } from 'vitest';
import { mockProject, server } from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import {
  useCreateProject,
  useDeleteProject,
  useInstallProject,
  useProject,
  useProjects,
  useUninstallProject,
} from './useProjects';

describe('useProjects', () => {
  it('fetches and returns the project list', async () => {
    const { result } = renderHook(() => useProjects(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockProject]);
  });

  it('reflects an error state when the request fails', async () => {
    server.use(http.get('/api/v1/projects', () => HttpResponse.error()));
    const { result } = renderHook(() => useProjects(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useProject', () => {
  it('fetches a single project by id', async () => {
    const { result } = renderHook(() => useProject('ws-1'), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe('ws-1');
  });

  it('does not fetch when id is empty', () => {
    const { result } = renderHook(() => useProject(''), {
      wrapper: createWrapper(),
    });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useCreateProject', () => {
  it('calls the create endpoint and returns the new project', async () => {
    const { result } = renderHook(() => useCreateProject(), {
      wrapper: createWrapper(),
    });
    result.current.mutate({ name: 'new-project' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.name).toBe('new-project');
  });

  it('enters error state when the request fails', async () => {
    server.use(
      http.post('/api/v1/projects', () =>
        HttpResponse.json({ error: 'conflict' }, { status: 409 }),
      ),
    );
    const { result } = renderHook(() => useCreateProject(), {
      wrapper: createWrapper(),
    });
    result.current.mutate({ name: 'bad' });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useInstallProject', () => {
  it('posts to the install endpoint and returns the queued job', async () => {
    server.use(
      http.post('/api/v1/projects/ws-1/install', () =>
        HttpResponse.json(
          {
            id: 'job-1',
            project_id: 'ws-1',
            type: 'env_install',
            status: 'pending',
          },
          { status: 202 },
        ),
      ),
    );
    const { result } = renderHook(() => useInstallProject('ws-1'), {
      wrapper: createWrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.type).toBe('env_install');
  });

  it('enters error state when install is rejected', async () => {
    server.use(
      http.post('/api/v1/projects/ws-1/install', () =>
        HttpResponse.json({ error: 'already in progress' }, { status: 409 }),
      ),
    );
    const { result } = renderHook(() => useInstallProject('ws-1'), {
      wrapper: createWrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useUninstallProject', () => {
  it('posts to the uninstall endpoint and returns the queued job', async () => {
    server.use(
      http.post('/api/v1/projects/ws-1/uninstall', () =>
        HttpResponse.json(
          {
            id: 'job-2',
            project_id: 'ws-1',
            type: 'env_uninstall',
            status: 'pending',
          },
          { status: 202 },
        ),
      ),
    );
    const { result } = renderHook(() => useUninstallProject('ws-1'), {
      wrapper: createWrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.type).toBe('env_uninstall');
  });
});

describe('useDeleteProject', () => {
  it('calls the delete endpoint successfully', async () => {
    const { result } = renderHook(() => useDeleteProject(), {
      wrapper: createWrapper(),
    });
    result.current.mutate('ws-1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});
