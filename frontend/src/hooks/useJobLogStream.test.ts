import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useJobLogStream } from './useJobLogStream';
import { useAuthStore } from '@/store/authStore';
import { mockUser } from '@/test/handlers';

// Minimal EventSource mock
class MockEventSource {
  static instances: MockEventSource[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: ((e: Event) => void) | null = null;
  private listeners: Record<string, EventListener> = {};
  readyState = 1;

  constructor(public url: string) {
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: EventListener) {
    this.listeners[type] = listener;
  }

  dispatchCustomEvent(type: string, data?: string) {
    const event = new MessageEvent(type, { data });
    this.listeners[type]?.(event as unknown as Event);
  }

  close = vi.fn();
}

let originalEventSource: typeof EventSource;

beforeEach(() => {
  MockEventSource.instances = [];
  originalEventSource = globalThis.EventSource;
  (globalThis as unknown as Record<string, unknown>).EventSource = MockEventSource;
  vi.spyOn(console, 'log').mockImplementation(() => {});
  vi.spyOn(console, 'error').mockImplementation(() => {});
  act(() => {
    useAuthStore.setState({ token: 'test-token', user: mockUser });
  });
});

afterEach(() => {
  (globalThis as unknown as Record<string, unknown>).EventSource = originalEventSource;
  act(() => {
    useAuthStore.setState({ token: null, user: null });
  });
  vi.restoreAllMocks();
});

describe('useJobLogStream', () => {
  it('does not open a stream for a completed job', () => {
    renderHook(() => useJobLogStream('job-1', 'completed'));
    expect(MockEventSource.instances).toHaveLength(0);
  });

  it('does not open a stream for a failed job', () => {
    renderHook(() => useJobLogStream('job-1', 'failed'));
    expect(MockEventSource.instances).toHaveLength(0);
  });

  it('opens an EventSource for a running job', () => {
    renderHook(() => useJobLogStream('job-1', 'running'));
    expect(MockEventSource.instances).toHaveLength(1);
    expect(MockEventSource.instances[0].url).toContain('/jobs/job-1/logs/stream');
  });

  it('opens an EventSource for a pending job', () => {
    renderHook(() => useJobLogStream('job-1', 'pending'));
    expect(MockEventSource.instances).toHaveLength(1);
  });

  it('includes the auth token in the stream URL', () => {
    renderHook(() => useJobLogStream('job-1', 'running'));
    expect(MockEventSource.instances[0].url).toContain('token=test-token');
  });

  it('appends incoming messages to logs', () => {
    const { result } = renderHook(() => useJobLogStream('job-1', 'running'));

    act(() => {
      MockEventSource.instances[0].onmessage?.(new MessageEvent('message', { data: 'line 1' }));
    });
    act(() => {
      MockEventSource.instances[0].onmessage?.(new MessageEvent('message', { data: 'line 2' }));
    });

    expect(result.current.logs).toContain('line 1');
    expect(result.current.logs).toContain('line 2');
  });

  it('initialises logs from the initialLogs prop', () => {
    const { result } = renderHook(() =>
      useJobLogStream('job-1', 'running', 'prior output\n')
    );
    expect(result.current.logs).toBe('prior output\n');
  });

  it('sets isStreaming to true for a running job', () => {
    const { result } = renderHook(() => useJobLogStream('job-1', 'running'));
    expect(result.current.isStreaming).toBe(true);
  });

  it('sets isStreaming to false when done event fires', () => {
    const { result } = renderHook(() => useJobLogStream('job-1', 'running'));

    act(() => {
      MockEventSource.instances[0].dispatchCustomEvent('done');
    });

    expect(result.current.isStreaming).toBe(false);
    expect(MockEventSource.instances[0].close).toHaveBeenCalled();
  });

  it('closes the EventSource on unmount', () => {
    const { unmount } = renderHook(() => useJobLogStream('job-1', 'running'));
    unmount();
    expect(MockEventSource.instances[0].close).toHaveBeenCalled();
  });

  it('does not open a stream when there is no auth token', () => {
    act(() => {
      useAuthStore.setState({ token: null, user: null });
    });
    renderHook(() => useJobLogStream('job-1', 'running'));
    expect(MockEventSource.instances).toHaveLength(0);
  });
});
