import { describe, expect, it } from 'vitest';
import { getWorkspaceVersionLabel } from './versions';

describe('getWorkspaceVersionLabel', () => {
  it('uses the manifest version when present', () => {
    expect(
      getWorkspaceVersionLabel({
        manifest_version: '0.0.3',
        version_number: 1,
      }),
    ).toBe('0.0.3');
  });

  it('falls back to the snapshot number', () => {
    expect(getWorkspaceVersionLabel({ version_number: 2 })).toBe('Snapshot 2');
  });
});
