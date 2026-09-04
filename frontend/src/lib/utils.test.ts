import { describe, expect, it } from 'vitest';
import { capitalize, cn, getWorkspaceVersionLabel } from './utils';

describe('cn', () => {
  it('merges class names', () => {
    expect(cn('foo', 'bar')).toBe('foo bar');
  });

  it('deduplicates conflicting Tailwind classes, keeping the last', () => {
    expect(cn('p-4', 'p-8')).toBe('p-8');
  });

  it('filters out falsy values', () => {
    const condition = false;
    expect(cn('foo', condition && 'bar', undefined, null, '')).toBe('foo');
  });

  it('handles conditional objects', () => {
    expect(cn({ 'text-red-500': true, 'text-blue-500': false })).toBe(
      'text-red-500',
    );
  });

  it('returns empty string when called with no arguments', () => {
    expect(cn()).toBe('');
  });
});

describe('capitalize', () => {
  it('capitalizes the first letter', () => {
    expect(capitalize('hello')).toBe('Hello');
  });

  it('leaves an already-capitalized string unchanged', () => {
    expect(capitalize('Hello')).toBe('Hello');
  });

  it('handles a single character', () => {
    expect(capitalize('a')).toBe('A');
  });

  it('returns empty string unchanged', () => {
    expect(capitalize('')).toBe('');
  });

  it('does not alter the rest of the string', () => {
    expect(capitalize('hELLO WORLD')).toBe('HELLO WORLD');
  });
});

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
