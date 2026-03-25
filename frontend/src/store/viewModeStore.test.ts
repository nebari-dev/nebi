import { describe, it, expect, beforeEach } from 'vitest';
import { useViewModeStore } from './viewModeStore';

beforeEach(() => {
  useViewModeStore.setState({ viewMode: 'local' });
  localStorage.clear();
});

describe('setViewMode', () => {
  it('updates viewMode to remote', () => {
    useViewModeStore.getState().setViewMode('remote');
    expect(useViewModeStore.getState().viewMode).toBe('remote');
  });

  it('updates viewMode back to local', () => {
    useViewModeStore.getState().setViewMode('remote');
    useViewModeStore.getState().setViewMode('local');
    expect(useViewModeStore.getState().viewMode).toBe('local');
  });
});

describe('initial state', () => {
  it('defaults to local view mode', () => {
    expect(useViewModeStore.getState().viewMode).toBe('local');
  });
});
