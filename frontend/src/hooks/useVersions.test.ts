import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import { useVersions, useVersion, useRollback } from './useVersions';

const mockVersion = {
  id: 'v-1',
  workspace_id: 'ws-1',
  version_number: 1,
  created_at: '2024-01-01T00:00:00Z',
  created_by: 'user-1',
  pixi_toml_hash: 'abc123',
  pixi_lock_hash: 'def456',
};

describe('useVersions', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/workspaces/:id/versions', () => HttpResponse.json([mockVersion]))
    );
  });

  it('fetches versions for a workspace', async () => {
    const { result } = renderHook(() => useVersions('ws-1'), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockVersion]);
  });

  it('does not fetch when environmentId is empty', () => {
    const { result } = renderHook(() => useVersions(''), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useVersion (single)', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/workspaces/:id/versions/:num', () => HttpResponse.json(mockVersion))
    );
  });

  it('fetches a single version', async () => {
    const { result } = renderHook(() => useVersion('ws-1', 1), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.version_number).toBe(1);
  });

  it('does not fetch when versionNumber is 0', () => {
    const { result } = renderHook(() => useVersion('ws-1', 0), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });

  it('does not fetch when environmentId is empty', () => {
    const { result } = renderHook(() => useVersion('', 1), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useRollback', () => {
  it('calls the rollback endpoint and succeeds', async () => {
    server.use(
      http.post('/api/v1/workspaces/:id/rollback', () =>
        HttpResponse.json({ id: 'job-2', status: 'pending' }, { status: 201 })
      )
    );
    const { result } = renderHook(() => useRollback('ws-1'), { wrapper: createWrapper() });
    result.current.mutate({ version_number: 1 });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });

  it('enters error state when rollback fails', async () => {
    server.use(
      http.post('/api/v1/workspaces/:id/rollback', () =>
        HttpResponse.json({ error: 'not found' }, { status: 404 })
      )
    );
    const { result } = renderHook(() => useRollback('ws-1'), { wrapper: createWrapper() });
    result.current.mutate({ version_number: 99 });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});
