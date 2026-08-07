import { renderHook, waitFor } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { describe, expect, it } from 'vitest';
import { server } from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import {
  useBuildEnvVars,
  useDeleteBuildEnvVar,
  useUpsertBuildEnvVar,
} from './useBuildEnv';

const mockBuildEnvVar = {
  id: 'env-1',
  key: 'GITLAB_TOKEN',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

describe('useBuildEnvVars', () => {
  it('fetches local build variables from the local endpoint', async () => {
    server.use(
      http.get('/api/v1/build-env-vars', () =>
        HttpResponse.json([mockBuildEnvVar]),
      ),
    );

    const { result } = renderHook(() => useBuildEnvVars('local'), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockBuildEnvVar]);
  });

  it('fetches remote build variables from the remote proxy endpoint', async () => {
    server.use(
      http.get('/api/v1/remote/build-env-vars', () =>
        HttpResponse.json([mockBuildEnvVar]),
      ),
    );

    const { result } = renderHook(() => useBuildEnvVars('remote'), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockBuildEnvVar]);
  });
});

describe('useUpsertBuildEnvVar', () => {
  it('writes remote build variables through the remote proxy endpoint', async () => {
    let requestBody: unknown;
    server.use(
      http.put('/api/v1/remote/build-env-vars', async ({ request }) => {
        requestBody = await request.json();
        return HttpResponse.json({
          ...mockBuildEnvVar,
          updated_at: '2024-01-02T00:00:00Z',
        });
      }),
    );

    const { result } = renderHook(() => useUpsertBuildEnvVar('remote'), {
      wrapper: createWrapper(),
    });

    result.current.mutate({
      key: 'GITLAB_TOKEN',
      value: 'secret-token',
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(requestBody).toEqual({
      key: 'GITLAB_TOKEN',
      value: 'secret-token',
    });
  });
});

describe('useDeleteBuildEnvVar', () => {
  it('deletes remote build variables through the remote proxy endpoint', async () => {
    let deletedKey = '';
    server.use(
      http.delete('/api/v1/remote/build-env-vars/:key', ({ params }) => {
        deletedKey = params.key as string;
        return new HttpResponse(null, { status: 204 });
      }),
    );

    const { result } = renderHook(() => useDeleteBuildEnvVar('remote'), {
      wrapper: createWrapper(),
    });

    result.current.mutate('GITLAB_TOKEN');

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(deletedKey).toBe('GITLAB_TOKEN');
  });
});
