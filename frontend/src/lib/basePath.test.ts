import { describe, it, expect, beforeEach } from 'vitest';
import { getBasePath, getApiBaseUrl } from './basePath';

describe('getBasePath', () => {
  beforeEach(() => {
    delete (window as Window & { __NEBI_BASE_PATH__?: string }).__NEBI_BASE_PATH__;
  });

  it('returns empty string when __NEBI_BASE_PATH__ is not set', () => {
    expect(getBasePath()).toBe('');
  });

  it('returns the configured base path', () => {
    (window as Window & { __NEBI_BASE_PATH__?: string }).__NEBI_BASE_PATH__ = '/nebi';
    expect(getBasePath()).toBe('/nebi');
  });
});

describe('getApiBaseUrl', () => {
  beforeEach(() => {
    delete (window as Window & { __NEBI_BASE_PATH__?: string }).__NEBI_BASE_PATH__;
  });

  it('returns /api/v1 when no base path is set', () => {
    expect(getApiBaseUrl()).toBe('/api/v1');
  });

  it('prepends the base path', () => {
    (window as Window & { __NEBI_BASE_PATH__?: string }).__NEBI_BASE_PATH__ = '/nebi';
    expect(getApiBaseUrl()).toBe('/nebi/api/v1');
  });
});
