import { getBasePath } from './basePath';

export type ThemeTokens = Record<string, string>;

export type BrandingConfig = {
  title?: string;
  logoUrl?: string;
  faviconUrl?: string;
  theme?: {
    light?: ThemeTokens;
    dark?: ThemeTokens;
  };
};

export type RuntimeBrandingConfig = {
  branding?: BrandingConfig;
};

const DEFAULT_TITLE = 'Nebi - Environment Management';
const DEFAULT_LOGO_URL = '/nebi-logo.svg';
const BRANDING_CONFIG_PATH = '/public/config.json';
const BRANDING_STYLE_ID = 'nebi-runtime-branding';
const UNSAFE_CSS_VALUE = /[;<>{}"'\\]|url\s*\(|expression\s*\(|javascript:/i;

let cachedConfig: RuntimeBrandingConfig | null = null;
let loadConfigPromise: Promise<RuntimeBrandingConfig> | null = null;

const normalizeString = (value: unknown): string | undefined => {
  if (typeof value !== 'string') {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed === '' ? undefined : trimmed;
};

const resolveBasePathUrl = (url: string): string => {
  const basePath = getBasePath();
  if (!url.startsWith('/') || url.startsWith('//') || basePath === '') {
    return url;
  }
  if (url === basePath || url.startsWith(`${basePath}/`)) {
    return url;
  }
  return `${basePath}${url}`;
};

const isSafeAssetUrl = (value: string): boolean => {
  if (value.startsWith('/')) {
    return !value.startsWith('//');
  }

  try {
    const parsed = new URL(value);
    return parsed.protocol === 'http:' || parsed.protocol === 'https:';
  } catch {
    return false;
  }
};

const toCssVariableName = (tokenName: string): string | undefined => {
  const trimmed = tokenName.trim();
  if (trimmed === '') {
    return undefined;
  }
  if (trimmed.startsWith('--')) {
    return trimmed;
  }
  const kebabCase = trimmed.replace(/([A-Z])/g, '-$1').toLowerCase();
  if (kebabCase === 'radius') {
    return '--radius';
  }
  if (kebabCase.startsWith('color-')) {
    return `--${kebabCase}`;
  }
  return `--color-${kebabCase}`;
};

const sanitizeThemeTokens = (tokens: unknown): ThemeTokens | undefined => {
  if (typeof tokens !== 'object' || tokens === null || Array.isArray(tokens)) {
    return undefined;
  }

  const compareKeys = (a: string, b: string): number => {
    if (a === b) {
      return 0;
    }
    return a < b ? -1 : 1;
  };
  // Sort keys to keep generated CSS deterministic across runs/environments.
  const tokenEntries = Object.entries(tokens).sort(([a], [b]) => compareKeys(a, b));
  const sanitizedEntries: Array<[string, string]> = [];
  for (const [tokenName, value] of tokenEntries) {
    const normalizedValue = normalizeString(value);
    if (!normalizedValue || UNSAFE_CSS_VALUE.test(normalizedValue)) {
      continue;
    }

    const cssVariable = toCssVariableName(tokenName);
    if (!cssVariable || !/^--[A-Za-z0-9_-]+$/.test(cssVariable)) {
      continue;
    }
    sanitizedEntries.push([cssVariable, normalizedValue]);
  }

  if (sanitizedEntries.length === 0) {
    return undefined;
  }

  return Object.fromEntries(sanitizedEntries);
};

const sanitizeBrandingConfig = (rawConfig: unknown): RuntimeBrandingConfig => {
  if (typeof rawConfig !== 'object' || rawConfig === null || Array.isArray(rawConfig)) {
    return {};
  }

  const configObj = rawConfig as Record<string, unknown>;
  const rawBranding =
    typeof configObj.branding === 'object' && configObj.branding !== null && !Array.isArray(configObj.branding)
      ? (configObj.branding as Record<string, unknown>)
      : configObj;
  const rawTheme = rawBranding.theme;
  const themeObject =
    typeof rawTheme === 'object' && rawTheme !== null && !Array.isArray(rawTheme)
      ? (rawTheme as Record<string, unknown>)
      : undefined;
  const light = sanitizeThemeTokens(themeObject?.light);
  const dark = sanitizeThemeTokens(themeObject?.dark);
  const title = normalizeString(rawBranding.title);
  const logoUrl = normalizeString(rawBranding.logoUrl);
  const faviconUrl = normalizeString(rawBranding.faviconUrl);
  const branding = title || logoUrl || faviconUrl || light || dark
    ? {
        title,
        logoUrl: logoUrl && isSafeAssetUrl(logoUrl) ? logoUrl : undefined,
        faviconUrl: faviconUrl && isSafeAssetUrl(faviconUrl) ? faviconUrl : undefined,
        theme: light || dark ? { light, dark } : undefined,
      }
    : undefined;

  return branding ? { branding } : {};
};

const buildCssBlock = (selector: ':root' | '.dark', tokens?: ThemeTokens): string => {
  if (!tokens) {
    return '';
  }
  const lines = Object.entries(tokens).map(([tokenName, value]) => `  ${tokenName}: ${value};`);
  if (lines.length === 0) {
    return '';
  }
  return `${selector} {\n${lines.join('\n')}\n}\n`;
};

const applyThemeStyles = (theme?: BrandingConfig['theme']): void => {
  const css = `${buildCssBlock(':root', theme?.light)}${buildCssBlock('.dark', theme?.dark)}`.trim();
  const existingStyle = document.getElementById(BRANDING_STYLE_ID);

  if (!css) {
    existingStyle?.remove();
    return;
  }

  const style = existingStyle ?? document.createElement('style');
  style.id = BRANDING_STYLE_ID;
  style.textContent = `${css}\n`;
  if (!existingStyle) {
    document.head.appendChild(style);
  }
};

const applyFavicon = (faviconUrl: string): void => {
  const resolvedUrl = resolveBasePathUrl(faviconUrl);
  const links = document.querySelectorAll<HTMLLinkElement>(
    'link[rel="icon"], link[rel="shortcut icon"], link[rel="apple-touch-icon"]',
  );
  for (const link of links) {
    link.href = resolvedUrl;
  }

  const fallbackLink = document.querySelector<HTMLLinkElement>('link[rel~="icon"]');
  if (!fallbackLink) {
    const link = document.createElement('link');
    link.rel = 'icon';
    link.href = resolvedUrl;
    document.head.appendChild(link);
  }
};

const applyBrandingConfig = (config: RuntimeBrandingConfig): void => {
  const branding = config.branding;
  document.title = branding?.title || DEFAULT_TITLE;

  if (branding?.faviconUrl) {
    applyFavicon(branding.faviconUrl);
  }

  applyThemeStyles(branding?.theme);
};

export const loadBrandingConfig = async (): Promise<RuntimeBrandingConfig> => {
  if (cachedConfig) {
    return cachedConfig;
  }
  if (loadConfigPromise) {
    return loadConfigPromise;
  }

  loadConfigPromise = (async () => {
    const configUrl = resolveBasePathUrl(BRANDING_CONFIG_PATH);
    try {
      const response = await fetch(configUrl, { cache: 'no-store' });
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      cachedConfig = sanitizeBrandingConfig(await response.json());
    } catch (error) {
      console.warn(`Failed to load branding config from ${configUrl}`, error);
      cachedConfig = {};
    }

    applyBrandingConfig(cachedConfig);
    return cachedConfig;
  })();

  try {
    return await loadConfigPromise;
  } finally {
    loadConfigPromise = null;
  }
};

export const getLogoUrl = (): string => {
  const configuredLogo = cachedConfig?.branding?.logoUrl;
  return resolveBasePathUrl(configuredLogo || DEFAULT_LOGO_URL);
};
