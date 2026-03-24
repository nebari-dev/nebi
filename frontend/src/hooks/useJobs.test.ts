import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server, mockJob } from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import { useJobs, useJob } from './useJobs';

describe('useJobs', () => {
  it('fetches and returns the job list', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([mockJob]);
  });

  it('reflects an error state when the request fails', async () => {
    server.use(
      http.get('/api/v1/jobs', () => HttpResponse.error())
    );
    const { result } = renderHook(() => useJobs(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useJob', () => {
  it('fetches a single job by id', async () => {
    const { result } = renderHook(() => useJob('job-1'), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe('job-1');
  });

  it('does not fetch when id is empty', () => {
    const { result } = renderHook(() => useJob(''), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });
});
