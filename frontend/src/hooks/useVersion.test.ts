import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import { useVersion } from './useVersion';

describe('useVersion', () => {
  it('fetches and returns version info', async () => {
    const { result } = renderHook(() => useVersion(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.version).toBe('0.0.1');
    expect(result.current.data?.mode).toBe('team');
  });

  it('reflects an error state when the request fails', async () => {
    server.use(
      http.get('/api/v1/version', () => HttpResponse.error())
    );
    const { result } = renderHook(() => useVersion(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});
