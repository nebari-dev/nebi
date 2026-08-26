import { act, renderHook, waitFor } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockJob, server } from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import type { Job } from '@/types';
import { useJob, useJobs } from './useJobs';

let restoreParent: (() => void) | undefined;

function job(overrides: Partial<Job>): Job {
  return {
    ...mockJob,
    type: 'env_install',
    status: 'running',
    ...overrides,
  };
}

function embedInParent() {
  const originalParent = window.parent;
  const parent = { postMessage: vi.fn() };

  Object.defineProperty(window, 'parent', {
    value: parent,
    configurable: true,
  });

  restoreParent = () => {
    Object.defineProperty(window, 'parent', {
      value: originalParent,
      configurable: true,
    });
  };

  return parent;
}

afterEach(() => {
  restoreParent?.();
  restoreParent = undefined;
});

describe('useJobs', () => {
  it('fetches and returns the job list', async () => {
    const { result } = renderHook(() => useJobs(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockJob]);
  });

  it('reflects an error state when the request fails', async () => {
    server.use(http.get('/api/v1/jobs', () => HttpResponse.error()));
    const { result } = renderHook(() => useJobs(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it('posts to the parent when a refresh-relevant job completes in an iframe', async () => {
    const parent = embedInParent();
    let status: Job['status'] = 'running';

    server.use(
      http.get('/api/v1/jobs', () => HttpResponse.json([job({ status })])),
    );

    const { result } = renderHook(() => useJobs(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.data?.[0].status).toBe(status));

    status = 'completed';
    await act(async () => {
      await result.current.refetch();
    });

    await waitFor(() => expect(parent.postMessage).toHaveBeenCalledTimes(1));
    expect(parent.postMessage).toHaveBeenCalledWith(
      {
        type: 'nebi:job-completed',
        jobType: 'env_install',
        workspace: 'ws-1',
      },
      window.location.origin,
    );

    await act(async () => {
      await result.current.refetch();
    });

    expect(parent.postMessage).toHaveBeenCalledTimes(1);
  });
});

describe('useJob', () => {
  it('fetches a single job by id', async () => {
    const { result } = renderHook(() => useJob('job-1'), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe('job-1');
  });

  it('does not fetch when id is empty', () => {
    const { result } = renderHook(() => useJob(''), {
      wrapper: createWrapper(),
    });
    expect(result.current.fetchStatus).toBe('idle');
  });
});
