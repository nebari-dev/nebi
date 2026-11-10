import { describe, expect, it } from 'vitest';
import { mockProjectItem } from '@/test/fixtures/contracts';
import {
  mockCompareRequested,
  mockCompareVersions,
} from '@/test/handlers/compare';
import type { CompareVersionMarks } from '@/types/client/compare';
import {
  buildCompareRows,
  defaultSelection,
  describeListDiff,
  formatVersionLabel,
  resolveSelection,
  versionMarks,
  versionNumbers,
} from './compareModel';

const marks = (
  installed: number | null,
  serverLatest: number | null,
  serverOnly: number[] = [],
): CompareVersionMarks => ({
  installed,
  serverLatest,
  serverOnly: new Set(serverOnly),
});

describe('formatVersionLabel', () => {
  it('labels installed and server-latest versions like the prototype', () => {
    const m = marks(7, 6, [8]);
    expect(formatVersionLabel(7, m)).toBe('v7 (installed)');
    expect(formatVersionLabel(6, m)).toBe('v6 (latest on server)');
    expect(formatVersionLabel(5, m)).toBe('v5');
    expect(formatVersionLabel(8, m)).toBe('v8 (on server)');
    expect(formatVersionLabel(5, marks(5, 5))).toBe(
      'v5 (installed, latest on server)',
    );
  });
});

describe('versionMarks', () => {
  it('prefers the project row and falls back to version markers', () => {
    expect(versionMarks(mockProjectItem, mockCompareVersions)).toMatchObject({
      installed: 7,
      serverLatest: 6,
    });
    const bare = {
      ...mockProjectItem,
      installed_version: null,
      server_latest_version: null,
    };
    expect(versionMarks(bare, mockCompareVersions)).toMatchObject({
      installed: 7,
      serverLatest: 6,
    });
    const withServerOnly = [
      { ...mockCompareVersions[0], number: 8, server_only: true },
      ...mockCompareVersions,
    ];
    expect(
      versionMarks(mockProjectItem, withServerOnly).serverOnly.has(8),
    ).toBe(true);
  });
});

describe('defaultSelection', () => {
  const numbers = [7, 6, 5, 4, 3, 2, 1];

  it('compares installed against the server latest', () => {
    expect(defaultSelection(numbers, marks(7, 6), null)).toEqual({
      base: 7,
      target: 6,
    });
  });

  it('starts from the version the page was opened from', () => {
    expect(defaultSelection(numbers, marks(7, 6), 5)).toEqual({
      base: 5,
      target: 7,
    });
    expect(defaultSelection(numbers, marks(7, 6), 7)).toEqual({
      base: 7,
      target: 6,
    });
  });

  it('falls back to the newest version the server also has', () => {
    expect(defaultSelection(numbers, marks(null, 5), null)).toEqual({
      base: 5,
      target: 7,
    });
    expect(defaultSelection(numbers, marks(null, null), null)).toEqual({
      base: 7,
      target: 6,
    });
  });

  it('ignores marks for versions that are not listed', () => {
    expect(defaultSelection([3, 2, 1], marks(9, 8), 12)).toEqual({
      base: 3,
      target: 2,
    });
  });

  it('handles one and zero versions', () => {
    expect(defaultSelection([1], marks(1, null), null)).toEqual({
      base: 1,
      target: 1,
    });
    expect(defaultSelection([], marks(null, null), null)).toBeNull();
  });
});

describe('resolveSelection', () => {
  const numbers = [7, 6, 5];
  it('reads base and to from the URL', () => {
    expect(
      resolveSelection(
        numbers,
        marks(7, 6),
        new URLSearchParams('base=5&to=6'),
      ),
    ).toEqual({ base: 5, target: 6, from: null });
  });
  it('falls back to defaults for unknown or malformed versions', () => {
    expect(
      resolveSelection(
        numbers,
        marks(7, 6),
        new URLSearchParams('base=99&to=abc&from=6'),
      ),
    ).toEqual({ base: 6, target: 7, from: 6 });
  });
});

describe('versionNumbers', () => {
  it('sorts newest first without duplicates', () => {
    const vs = [
      mockCompareVersions[3],
      mockCompareVersions[0],
      mockCompareVersions[0],
    ];
    expect(versionNumbers(vs)).toEqual([7, 4]);
  });
});

describe('buildCompareRows', () => {
  it('numbers each side only for its own packages', () => {
    const rows = buildCompareRows(mockCompareRequested);
    expect(rows.map((r) => [r.name, r.base?.line, r.compare?.line])).toEqual([
      ['ipykernel', 1, 1],
      ['numpy', 2, 2],
      ['python', 3, 3],
      ['rich', undefined, 4],
      ['scipy', 4, undefined],
    ]);
    expect(rows[3].compare).toMatchObject({
      source: 'pypi',
      version: '13.9.4',
    });
    expect(rows[1].base).toMatchObject({
      constraint: '>=2.1',
      version: '2.1.3',
    });
  });
});

describe('buildCompareRows constraints', () => {
  it('shows the constraint only when the two sides request different ones', () => {
    const rows = buildCompareRows([
      {
        name: 'numpy',
        source: 'conda',
        change: 'changed',
        base: { constraint: '>=2.0', version: '2.5.4' },
        compare: { constraint: '>=2.1', version: '2.5.4' },
      },
      {
        name: 'python',
        source: 'conda',
        change: 'changed',
        base: { constraint: '3.12.*', version: '3.12.16' },
        compare: { constraint: '3.12.*', version: '3.12.17' },
      },
      {
        name: 'rich',
        source: 'conda',
        change: 'added',
        base: null,
        compare: { constraint: '>=13', version: '13.9.4' },
      },
    ]);
    expect(rows[0].base?.showConstraint).toBe(true);
    expect(rows[0].compare?.showConstraint).toBe(true);
    expect(rows[1].base?.showConstraint).toBeUndefined();
    expect(rows[2].compare?.showConstraint).toBeUndefined();
  });
});

describe('describeListDiff', () => {
  it('describes added and removed entries', () => {
    expect(
      describeListDiff({ added: ['bioconda'], removed: ['defaults'] }),
    ).toBe('added bioconda; removed defaults');
    expect(describeListDiff({ added: null, removed: [] })).toBe('');
  });
});
