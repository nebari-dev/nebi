import { describe, expect, it } from 'vitest';
import { capitalize } from './strings';

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
