import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { THEME_STORAGE_KEY } from '@/lib/theme';
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

const setMatchMedia = (matches: boolean) => {
  let listener: ((event: MediaQueryListEvent) => void) | undefined;
  const state = { matches };

  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation((media: string) => ({
      get matches() {
        return state.matches;
      },
      media,
      onchange: null,
      addEventListener: vi.fn(
        (_event: string, handler: (event: MediaQueryListEvent) => void) => {
          listener = handler;
        },
      ),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  );

  return {
    change(nextMatches: boolean) {
      state.matches = nextMatches;
      listener?.({ matches: nextMatches } as MediaQueryListEvent);
    },
  };
};

const renderThemePreference = () =>
  renderHook(() => useThemePreference({ storageKey: THEME_STORAGE_KEY }));

describe('useThemePreference (nebi storage key)', () => {
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

  it('restores a preference saved under the legacy nebi key', () => {
    storedValues[THEME_STORAGE_KEY] = 'dark';

    const { result } = renderThemePreference();

    expect(result.current.themeMode).toBe('dark');
    expect(result.current.isDarkMode).toBe(true);
  });

  it('persists explicit theme choices under the nebi key and applies them to the root element', async () => {
    const { result } = renderThemePreference();

    act(() => result.current.setThemeMode('dark'));

    expect(result.current.themeMode).toBe('dark');
    expect(result.current.isDarkMode).toBe(true);
    expect(storedValues[THEME_STORAGE_KEY]).toBe('dark');

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
