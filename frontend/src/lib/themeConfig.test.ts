import { beforeEach, describe, expect, it, vi } from 'vitest';

type BasePathWindow = Window & { __NEBI_BASE_PATH__?: string };

const defaultHead = `
  <link rel="icon" href="/favicon.ico" />
  <link rel="shortcut icon" href="/favicon.ico" />
  <link rel="apple-touch-icon" href="/apple-touch-icon.png" />
`;

describe('themeConfig', () => {
  beforeEach(() => {
    vi.resetModules();
    vi.unstubAllGlobals();
    delete (window as BasePathWindow).__NEBI_BASE_PATH__;
    document.head.innerHTML = defaultHead;
    document.title = 'Initial Title';
  });

  it('loads /public/config.json, applies runtime config, and resolves logo URL', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        title: 'Acme Nebi',
        logoUrl: '/assets/acme-logo.svg',
        faviconUrl: '/assets/acme-favicon.ico',
        theme: {
          light: { primary: '#123456', navHover: '#eef3ff', '': '#ffffff' },
          dark: { primary: '#89abef' },
        },
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const { loadThemeConfig, getLogoUrl } = await import('./themeConfig');
    const config = await loadThemeConfig();

    expect(fetchMock).toHaveBeenCalledWith('/public/config.json', { cache: 'no-store' });
    expect(document.title).toBe('Acme Nebi');
    expect(getLogoUrl()).toBe('/assets/acme-logo.svg');
    expect(config.title).toBe('Acme Nebi');

    const favicon = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    expect(favicon?.getAttribute('href')).toBe('/assets/acme-favicon.ico');

    const style = document.getElementById('nebi-runtime-theme');
    expect(style?.textContent).toContain('--color-primary: #123456;');
    expect(style?.textContent).toContain('--color-nav-hover: #eef3ff;');
    expect(style?.textContent).not.toContain('--color-: #ffffff;');
    expect(style?.textContent).toContain('.dark');
  });

  it('prepends base path for config fetch and root-relative assets', async () => {
    (window as BasePathWindow).__NEBI_BASE_PATH__ = '/nebi';
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        logoUrl: '/brand/logo.svg',
        faviconUrl: '/brand/favicon.ico',
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const { loadThemeConfig, getLogoUrl } = await import('./themeConfig');
    await loadThemeConfig();

    expect(fetchMock).toHaveBeenCalledWith('/nebi/public/config.json', { cache: 'no-store' });
    expect(getLogoUrl()).toBe('/nebi/brand/logo.svg');

    const favicon = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    expect(favicon?.getAttribute('href')).toBe('/nebi/brand/favicon.ico');
  });

  it('falls back to defaults when config cannot be loaded', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    const fetchMock = vi.fn().mockRejectedValue(new Error('boom'));
    vi.stubGlobal('fetch', fetchMock);

    const { loadThemeConfig, getLogoUrl } = await import('./themeConfig');
    const config = await loadThemeConfig();

    expect(config).toEqual({});
    expect(document.title).toBe('Nebi - Environment Management');
    expect(getLogoUrl()).toBe('/nebi-logo.svg');
    expect(warnSpy).toHaveBeenCalled();
  });

  it('ignores unsafe asset URLs and falls back to defaults', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        title: 'Unsafe assets',
        logoUrl: 'javascript:alert(1)',
        faviconUrl: 'data:image/svg+xml;base64,PHN2Zy8+',
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const { loadThemeConfig, getLogoUrl } = await import('./themeConfig');
    await loadThemeConfig();

    expect(getLogoUrl()).toBe('/nebi-logo.svg');
    expect(document.title).toBe('Unsafe assets');
    const favicon = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    expect(favicon?.getAttribute('href')).toBe('/favicon.ico');
  });

  it('ignores route-relative asset URLs to avoid path-dependent resolution', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        logoUrl: 'brand/logo.svg',
        faviconUrl: 'brand/favicon.ico',
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const { loadThemeConfig, getLogoUrl } = await import('./themeConfig');
    await loadThemeConfig();

    expect(getLogoUrl()).toBe('/nebi-logo.svg');
    const favicon = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    expect(favicon?.getAttribute('href')).toBe('/favicon.ico');
  });
});
