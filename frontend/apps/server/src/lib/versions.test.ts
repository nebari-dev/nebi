import { describe, expect, it } from 'vitest';
import { getProjectVersionLabel } from './versions';

describe('getProjectVersionLabel', () => {
  it('uses the manifest version when present', () => {
    expect(
      getProjectVersionLabel({
        manifest_version: '0.0.3',
        version_number: 1,
      }),
    ).toBe('0.0.3');
  });

  it('falls back to the snapshot number', () => {
    expect(getProjectVersionLabel({ version_number: 2 })).toBe('Snapshot 2');
  });
});
