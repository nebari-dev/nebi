import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  THEME_MODE_STORAGE_KEY,
  useThemePreference,
} from './useThemePreference';

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

  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation((media: string) => ({
      matches,
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
      listener?.({ matches: nextMatches } as MediaQueryListEvent);
    },
  };
};

describe('useThemePreference', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    setLocalStorage();
    document.documentElement.classList.remove('dark');
    document.documentElement.style.colorScheme = '';
    setMatchMedia(false);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('defaults to the system preference when no theme is stored', async () => {
    setMatchMedia(true);

    const { result } = renderHook(() => useThemePreference());

    expect(result.current.themeMode).toBe('system');
    expect(result.current.isDarkMode).toBe(true);

    await waitFor(() => {
      expect(document.documentElement).toHaveClass('dark');
      expect(storedValues[THEME_MODE_STORAGE_KEY]).toBe('system');
    });
  });

  it('persists explicit theme choices and applies them to the root element', async () => {
    const { result } = renderHook(() => useThemePreference());

    act(() => result.current.setThemeMode('dark'));

    expect(result.current.themeMode).toBe('dark');
    expect(result.current.isDarkMode).toBe(true);

    await waitFor(() => {
      expect(document.documentElement).toHaveClass('dark');
      expect(storedValues[THEME_MODE_STORAGE_KEY]).toBe('dark');
    });
  });

  it('updates system mode when the OS preference changes', async () => {
    const media = setMatchMedia(false);
    const { result } = renderHook(() => useThemePreference());

    expect(result.current.isDarkMode).toBe(false);

    act(() => media.change(true));

    expect(result.current.themeMode).toBe('system');
    expect(result.current.isDarkMode).toBe(true);

    await waitFor(() => {
      expect(document.documentElement).toHaveClass('dark');
    });
  });
});
