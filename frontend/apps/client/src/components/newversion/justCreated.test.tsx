import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  markJustCreated,
  resetJustCreated,
  useJustCreated,
} from './justCreated';

describe('useJustCreated', () => {
  afterEach(() => {
    resetJustCreated();
    vi.useRealTimers();
  });

  it('lights up the version just created, then fades', () => {
    vi.useFakeTimers();
    markJustCreated('p1', 8);
    const { result } = renderHook(() => useJustCreated('p1', 8));
    expect(result.current).toBe('lit');
    act(() => {
      vi.advanceTimersByTime(600);
    });
    expect(result.current).toBe('fading');
    act(() => {
      vi.advanceTimersByTime(1200);
    });
    expect(result.current).toBeNull();
  });

  it('leaves other versions and projects alone', () => {
    markJustCreated('p1', 8);
    expect(renderHook(() => useJustCreated('p1', 7)).result.current).toBeNull();
    expect(renderHook(() => useJustCreated('p2', 8)).result.current).toBeNull();
    expect(
      renderHook(() => useJustCreated(undefined, 8)).result.current,
    ).toBeNull();
  });

  it('does not light up a row shown long after creation', () => {
    vi.useFakeTimers();
    markJustCreated('p1', 8);
    vi.advanceTimersByTime(2500);
    expect(renderHook(() => useJustCreated('p1', 8)).result.current).toBeNull();
  });
});
