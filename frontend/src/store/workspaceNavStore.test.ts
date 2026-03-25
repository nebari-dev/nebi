import { describe, it, expect, beforeEach } from 'vitest';
import { useWorkspaceNavStore } from './workspaceNavStore';

beforeEach(() => {
  useWorkspaceNavStore.setState({ pendingTab: null });
});

describe('setPendingTab', () => {
  it('sets the pending tab', () => {
    useWorkspaceNavStore.getState().setPendingTab('versions');
    expect(useWorkspaceNavStore.getState().pendingTab).toBe('versions');
  });
});

describe('consumePendingTab', () => {
  it('returns the pending tab and clears it', () => {
    useWorkspaceNavStore.getState().setPendingTab('jobs');
    const result = useWorkspaceNavStore.getState().consumePendingTab();
    expect(result).toBe('jobs');
    expect(useWorkspaceNavStore.getState().pendingTab).toBeNull();
  });

  it('returns null when no tab is pending', () => {
    const result = useWorkspaceNavStore.getState().consumePendingTab();
    expect(result).toBeNull();
  });

  it('only consumes once — second call returns null', () => {
    useWorkspaceNavStore.getState().setPendingTab('packages');
    useWorkspaceNavStore.getState().consumePendingTab();
    expect(useWorkspaceNavStore.getState().consumePendingTab()).toBeNull();
  });
});
