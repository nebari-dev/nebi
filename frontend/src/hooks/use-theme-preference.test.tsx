import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { THEME_MODE_STORAGE_KEY } from '@/lib/theme';
import { useThemePreference } from './use-theme-preference';

let storedValues: Record<string, string>;

const setLocalStorage = () => {
  storedValues = {};
  const localStorage = {
    getItem: vi.fn((key: string) => storedValues[key] ?? null),
    setItem: vi.fn((key: string, value: string) => {
      storedValues[key] = value;
    }),
    removeItem: vi.fn((key: string) => {
      delete storedValues[key];
    }),
    clear: vi.fn(() => {
      storedValues = {};
    }),
  };

  vi.stubGlobal('localStorage', localStorage);
};

const setMatchMedia = (initialMatches: boolean) => {
  let matches = initialMatches;
  const listeners = new Set<(event: MediaQueryListEvent) => void>();

  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation((media: string) => ({
      get matches() {
        return matches;
      },
      media,
      onchange: null,
      addEventListener: vi.fn(
        (_event: string, handler: (event: MediaQueryListEvent) => void) => {
          listeners.add(handler);
        },
      ),
      removeEventListener: vi.fn(
        (_event: string, handler: (event: MediaQueryListEvent) => void) => {
          listeners.delete(handler);
        },
      ),
      dispatchEvent: vi.fn(),
    })),
  );

  return {
    change(nextMatches: boolean) {
      matches = nextMatches;
      for (const listener of listeners) {
        listener({ matches: nextMatches } as MediaQueryListEvent);
      }
    },
  };
};

const renderThemePreference = () =>
  renderHook(() => useThemePreference({ storageKey: THEME_MODE_STORAGE_KEY }));

describe('useThemePreference', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    setLocalStorage();
    document.documentElement.classList.remove('dark');
    setMatchMedia(false);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('defaults to the system preference when no theme is stored', async () => {
    setMatchMedia(true);

    const { result } = renderThemePreference();

    expect(result.current.themeMode).toBe('system');
    expect(result.current.isDarkMode).toBe(true);

    await waitFor(() => {
      expect(document.documentElement).toHaveClass('dark');
    });
  });

  it('reads a previously stored preference from the nebi storage key', () => {
    storedValues[THEME_MODE_STORAGE_KEY] = 'dark';

    const { result } = renderThemePreference();

    expect(result.current.themeMode).toBe('dark');
    expect(result.current.isDarkMode).toBe(true);
  });

  it('persists explicit theme choices and applies them to the root element', async () => {
    const { result } = renderThemePreference();

    act(() => result.current.setThemeMode('dark'));

    expect(result.current.themeMode).toBe('dark');
    expect(result.current.isDarkMode).toBe(true);
    expect(storedValues[THEME_MODE_STORAGE_KEY]).toBe('dark');

    await waitFor(() => {
      expect(document.documentElement).toHaveClass('dark');
    });
  });

  it('updates system mode when the OS preference changes', async () => {
    const media = setMatchMedia(false);
    const { result } = renderThemePreference();

    expect(result.current.isDarkMode).toBe(false);

    act(() => media.change(true));

    expect(result.current.themeMode).toBe('system');
    expect(result.current.isDarkMode).toBe(true);

    await waitFor(() => {
      expect(document.documentElement).toHaveClass('dark');
    });
  });
});
