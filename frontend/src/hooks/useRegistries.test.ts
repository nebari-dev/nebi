import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server, mockRegistry, mockPublishDefaults, mockPublication, mockJob } from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import {
  usePublicRegistries,
  useRegistries,
  usePublishDefaults,
  usePublications,
  usePublishWorkspace,
  useCreateRegistry,
  useDeleteRegistry,
} from './useRegistries';

describe('usePublicRegistries', () => {
  it('fetches the public registries list', async () => {
    const { result } = renderHook(() => usePublicRegistries(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockRegistry]);
  });
});

describe('useRegistries', () => {
  it('fetches the admin registries list', async () => {
    server.use(
      http.get('/api/v1/admin/registries', () => HttpResponse.json([mockRegistry]))
    );
    const { result } = renderHook(() => useRegistries(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockRegistry]);
  });

  it('reflects an error state when the request fails', async () => {
    server.use(
      http.get('/api/v1/admin/registries', () => HttpResponse.error())
    );
    const { result } = renderHook(() => useRegistries(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('usePublishDefaults', () => {
  it('fetches publish defaults for a workspace', async () => {
    const { result } = renderHook(() => usePublishDefaults('ws-1'), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(mockPublishDefaults);
  });

  it('does not fetch when workspaceId is empty', () => {
    const { result } = renderHook(() => usePublishDefaults(''), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('usePublications', () => {
  it('fetches publications for a workspace', async () => {
    const { result } = renderHook(() => usePublications('ws-1'), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockPublication]);
  });

  it('does not fetch when workspaceId is empty', () => {
    const { result } = renderHook(() => usePublications(''), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('usePublishWorkspace', () => {
  it('calls the publish endpoint and returns a job', async () => {
    const { result } = renderHook(() => usePublishWorkspace(), { wrapper: createWrapper() });
    result.current.mutate({
      workspaceId: 'ws-1',
      data: { registry_id: 'reg-1', namespace: 'myorg', repository: 'myenv', tag: 'latest' },
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toMatchObject({ id: mockJob.id });
  });

  it('enters error state when publish fails', async () => {
    server.use(
      http.post('/api/v1/workspaces/:id/publish', () =>
        HttpResponse.json({ error: 'registry unreachable' }, { status: 502 })
      )
    );
    const { result } = renderHook(() => usePublishWorkspace(), { wrapper: createWrapper() });
    result.current.mutate({
      workspaceId: 'ws-1',
      data: { registry_id: 'reg-1', namespace: 'myorg', repository: 'myenv', tag: 'latest' },
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useCreateRegistry', () => {
  it('calls the create registry endpoint', async () => {
    server.use(
      http.post('/api/v1/admin/registries', () =>
        HttpResponse.json(mockRegistry, { status: 201 })
      )
    );
    const { result } = renderHook(() => useCreateRegistry(), { wrapper: createWrapper() });
    result.current.mutate({
      name: 'New Registry',
      url: 'https://new.registry.io',
      username: 'user',
      password: 'pass',
      namespace: 'org',
      is_default: false,
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useDeleteRegistry', () => {
  it('calls the delete registry endpoint', async () => {
    server.use(
      http.delete('/api/v1/admin/registries/:id', () =>
        new HttpResponse(null, { status: 204 })
      )
    );
    const { result } = renderHook(() => useDeleteRegistry(), { wrapper: createWrapper() });
    result.current.mutate('reg-1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});
