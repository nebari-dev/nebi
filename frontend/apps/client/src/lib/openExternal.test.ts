import { afterEach, describe, expect, it, vi } from 'vitest';
import { openExternal } from './openExternal';

describe('openExternal', () => {
  afterEach(() => {
    delete window.runtime;
    vi.restoreAllMocks();
  });

  it('uses the Wails runtime when available', () => {
    const browserOpenUrl = vi.fn();
    window.runtime = { BrowserOpenURL: browserOpenUrl };
    const windowOpen = vi.spyOn(window, 'open').mockImplementation(() => null);

    openExternal('https://example.com/docs');

    expect(browserOpenUrl).toHaveBeenCalledWith('https://example.com/docs');
    expect(windowOpen).not.toHaveBeenCalled();
  });

  it('falls back to a noopener browser tab outside Wails', () => {
    const windowOpen = vi.spyOn(window, 'open').mockImplementation(() => null);

    openExternal('https://example.com/docs');

    expect(windowOpen).toHaveBeenCalledWith(
      'https://example.com/docs',
      '_blank',
      'noopener,noreferrer',
    );
  });
});
