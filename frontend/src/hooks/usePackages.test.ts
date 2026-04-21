import { describe, it, expect, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/handlers';
import { createWrapper } from '@/test/utils';
import { usePackages } from './usePackages';

const mockPackages = [{ name: 'numpy', version: '1.26.0', platform: 'linux-64' }];

describe('usePackages', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/workspaces/:id/packages', () => HttpResponse.json(mockPackages))
    );
  });

  it('fetches packages for a workspace', async () => {
    const { result } = renderHook(() => usePackages('ws-1'), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(mockPackages);
  });

  it('does not fetch when environmentId is empty', () => {
    const { result } = renderHook(() => usePackages(''), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
  });

  it('reflects an error state when the request fails', async () => {
    server.use(
      http.get('/api/v1/workspaces/:id/packages', () => HttpResponse.error())
    );
    const { result } = renderHook(() => usePackages('ws-1'), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});
