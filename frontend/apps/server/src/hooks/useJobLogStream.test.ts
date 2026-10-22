import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useAuthStore } from '@/store/authStore';
import { mockUser } from '@/test/handlers';
import { useJobLogStream } from './useJobLogStream';

const encoder = new TextEncoder();

const streamResponse = (body: string, init: ResponseInit = {}) => {
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(encoder.encode(body));
      controller.close();
    },
  });
  return new Response(stream, {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
    ...init,
  });
};

describe('useJobLogStream', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn(() => new Promise<Response>(() => {}));
    vi.stubGlobal('fetch', fetchMock);
    vi.spyOn(console, 'error').mockImplementation(() => {});
    act(() => {
      useAuthStore.setState({ token: 'test-token', user: mockUser });
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    act(() => {
      useAuthStore.setState({ token: null, user: null });
    });
    vi.restoreAllMocks();
  });

  it('does not open a stream for a completed job', () => {
    renderHook(() => useJobLogStream('job-1', 'completed'));
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('does not open a stream for a failed job', () => {
    renderHook(() => useJobLogStream('job-1', 'failed'));
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('opens a fetch stream for a running job', async () => {
    renderHook(() => useJobLogStream('job-1', 'running'));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0][0]).toContain('/jobs/job-1/logs/stream');
  });

  it('opens a fetch stream for a pending job', async () => {
    renderHook(() => useJobLogStream('job-1', 'pending'));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  });

  it('sends the auth token in a header instead of the stream URL', async () => {
    renderHook(() => useJobLogStream('job-1', 'running'));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const [url, options] = fetchMock.mock.calls[0] as [string, RequestInit];

    expect(url).not.toContain('token=');
    expect(options.headers).toMatchObject({
      Accept: 'text/event-stream',
      Authorization: 'Bearer test-token',
    });
  });

  it('appends incoming messages to logs', async () => {
    fetchMock.mockResolvedValueOnce(
      streamResponse('data: line 1\n\ndata: line 2\n\n'),
    );

    const { result } = renderHook(() => useJobLogStream('job-1', 'running'));

    await waitFor(() => expect(result.current.logs).toContain('line 1'));
    expect(result.current.logs).toContain('line 2');
  });

  it('preserves multiline data fields', async () => {
    fetchMock.mockResolvedValueOnce(
      streamResponse('data: line 1\ndata: line 2\n\n'),
    );

    const { result } = renderHook(() => useJobLogStream('job-1', 'running'));

    await waitFor(() => expect(result.current.logs).toBe('line 1\nline 2\n'));
  });

  it('initialises logs from the initialLogs prop', () => {
    const { result } = renderHook(() =>
      useJobLogStream('job-1', 'running', 'prior output\n'),
    );
    expect(result.current.logs).toBe('prior output\n');
  });

  it('sets isStreaming to true for a running job', async () => {
    const { result } = renderHook(() => useJobLogStream('job-1', 'running'));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(result.current.isStreaming).toBe(true);
  });

  it('sets isStreaming to false when done event arrives', async () => {
    fetchMock.mockResolvedValueOnce(
      streamResponse('event: done\ndata: Job completed\n\n'),
    );

    const { result } = renderHook(() => useJobLogStream('job-1', 'running'));

    await waitFor(() => expect(result.current.isStreaming).toBe(false));
    expect(result.current.logs).not.toContain('Job completed');
  });

  it('aborts the fetch stream on unmount', async () => {
    const { unmount } = renderHook(() => useJobLogStream('job-1', 'running'));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const options = fetchMock.mock.calls[0][1] as RequestInit;
    const signal = options.signal as AbortSignal;

    expect(signal.aborted).toBe(false);
    unmount();
    expect(signal.aborted).toBe(true);
  });

  it('does not open a stream when there is no auth token', () => {
    act(() => {
      useAuthStore.setState({ token: null, user: null });
    });
    renderHook(() => useJobLogStream('job-1', 'running'));
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
