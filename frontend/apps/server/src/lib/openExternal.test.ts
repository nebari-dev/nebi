import { afterEach, describe, expect, it, vi } from 'vitest';
import { openExternal } from './openExternal';

describe('openExternal', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('opens a noopener browser tab', () => {
    const windowOpen = vi.spyOn(window, 'open').mockImplementation(() => null);

    openExternal('https://example.com/docs');

    expect(windowOpen).toHaveBeenCalledWith(
      'https://example.com/docs',
      '_blank',
      'noopener,noreferrer',
    );
  });
});
