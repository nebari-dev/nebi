import { useEffect, useState } from 'react';
import { useLocalStorageState } from './useLocalStorageState';

const THEME_MODES = ['light', 'dark', 'system'] as const;
export type ThemeMode = (typeof THEME_MODES)[number];

export const THEME_MODE_STORAGE_KEY = 'nebi:themeMode';

const isThemeMode = (value: string): value is ThemeMode =>
  (THEME_MODES as readonly string[]).includes(value);

const prefersDark = (): boolean => {
  try {
    return window.matchMedia('(prefers-color-scheme: dark)').matches;
  } catch {
    return false;
  }
};

const readStoredThemeMode = (stored: string | null): ThemeMode => {
  if (stored && isThemeMode(stored)) {
    return stored;
  }

  return 'system';
};

export const useThemePreference = () => {
  const [themeMode, setThemeMode] = useLocalStorageState<ThemeMode>(
    THEME_MODE_STORAGE_KEY,
    readStoredThemeMode,
  );
  const [systemPrefersDark, setSystemPrefersDark] =
    useState<boolean>(prefersDark);

  useEffect(() => {
    let mediaQuery: MediaQueryList;
    try {
      mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    } catch {
      return;
    }

    setSystemPrefersDark(mediaQuery.matches);

    const handleChange = (event: MediaQueryListEvent) => {
      setSystemPrefersDark(event.matches);
    };

    mediaQuery.addEventListener('change', handleChange);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }, []);

  const isDarkMode =
    themeMode === 'system' ? systemPrefersDark : themeMode === 'dark';

  useEffect(() => {
    document.documentElement.classList.toggle('dark', isDarkMode);
  }, [isDarkMode]);

  return { themeMode, isDarkMode, setThemeMode };
};
