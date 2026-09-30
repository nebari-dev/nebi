import { beforeEach, describe, expect, it } from 'vitest';
import { useProjectNavStore } from './projectNavStore';

beforeEach(() => {
  useProjectNavStore.setState({ pendingTab: null });
});

describe('setPendingTab', () => {
  it('sets the pending tab', () => {
    useProjectNavStore.getState().setPendingTab('versions');
    expect(useProjectNavStore.getState().pendingTab).toBe('versions');
  });
});

describe('consumePendingTab', () => {
  it('returns the pending tab and clears it', () => {
    useProjectNavStore.getState().setPendingTab('jobs');
    const result = useProjectNavStore.getState().consumePendingTab();
    expect(result).toBe('jobs');
    expect(useProjectNavStore.getState().pendingTab).toBeNull();
  });

  it('returns null when no tab is pending', () => {
    const result = useProjectNavStore.getState().consumePendingTab();
    expect(result).toBeNull();
  });

  it('only consumes once — second call returns null', () => {
    useProjectNavStore.getState().setPendingTab('packages');
    useProjectNavStore.getState().consumePendingTab();
    expect(useProjectNavStore.getState().consumePendingTab()).toBeNull();
  });
});
