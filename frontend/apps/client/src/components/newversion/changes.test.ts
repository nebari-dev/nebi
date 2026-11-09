import { describe, expect, it } from 'vitest';
import { mockChanges, mockNoChanges } from '@/test/handlers/newversion';
import type { ManifestForm } from '@/types/contracts';
import {
  hasChanges,
  newVersionPath,
  sameForm,
  suggestDescription,
  summarizeRequested,
  tagError,
  tagsForRequest,
  versionList,
} from './changes';

describe('new version helpers', () => {
  it('summarizes requested changes in the prototype wording', () => {
    expect(summarizeRequested(mockChanges)).toEqual([
      'added pandas',
      'removed numpy',
      'changed python',
    ]);
    expect(suggestDescription(mockChanges)).toBe(
      'Added pandas, removed numpy, changed python',
    );
    expect(suggestDescription(mockNoChanges)).toBe('');
    expect(suggestDescription(undefined)).toBe('');
  });

  it('only counts constraint changes as changed packages', () => {
    const resolvedOnly = {
      ...mockNoChanges,
      requested: [
        {
          name: 'numpy',
          source: 'conda' as const,
          change: 'changed' as const,
          base: { constraint: '>=2.0', version: '2.1.0' },
          compare: { constraint: '>=2.0', version: '2.2.0' },
        },
      ],
    };
    expect(summarizeRequested(resolvedOnly)).toEqual([]);
  });

  it('joins names like the prototype', () => {
    const many = {
      ...mockNoChanges,
      requested: ['a', 'b', 'c'].map((name) => ({
        name,
        source: 'conda' as const,
        change: 'added' as const,
        base: null,
        compare: { constraint: '*', version: '*' },
      })),
    };
    expect(summarizeRequested(many)).toEqual(['added a, b and c']);
    expect(versionList([6])).toBe('version 6');
    expect(versionList([6, 7])).toBe('versions 6 and 7');
    expect(versionList([5, 6, 7])).toBe('versions 5, 6 and 7');
  });

  it('detects changes in packages, channels and platforms', () => {
    expect(hasChanges(mockNoChanges)).toBe(false);
    expect(hasChanges(mockChanges)).toBe(true);
    expect(
      hasChanges({
        ...mockNoChanges,
        platforms: { added: ['osx-arm64'], removed: [] },
      }),
    ).toBe(true);
    expect(hasChanges(undefined)).toBe(false);
  });

  it('validates tags like the prototype', () => {
    expect(tagError('stable')).toBeNull();
    expect(tagError('v1.2_rc-1')).toBeNull();
    expect(tagError('1x')).toBe(
      '“1x” can’t be used: tags must start with a letter.',
    );
    expect(tagError('a b')).toBe(
      '“a b” can’t be used: tags use letters, numbers, dots, hyphens and underscores.',
    );
    expect(tagsForRequest(['latest', 'stable'])).toEqual(['stable']);
  });

  it('compares forms and builds paths', () => {
    const f: ManifestForm = {
      name: 'p',
      channels: ['conda-forge'],
      platforms: ['linux-64'],
      requested: [{ name: 'numpy', constraint: '*' }],
    };
    expect(
      sameForm(f, {
        ...f,
        requested: [{ name: 'numpy', constraint: '*', source: 'conda' }],
      }),
    ).toBe(true);
    expect(sameForm(f, { ...f, platforms: ['osx-arm64'] })).toBe(false);
    expect(newVersionPath('id', 5)).toBe('/projects/id/new-version?from=5');
  });
});
