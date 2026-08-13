import { useEffect, useState } from 'react';

const getStoredValue = (key: string): string | null => {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
};

const setStoredValue = (key: string, value: string): void => {
  try {
    localStorage.setItem(key, value);
  } catch {
    console.error(`Failed to persist "${key}" to localStorage`);
  }
};

export const useLocalStorageState = <T extends string>(
  key: string,
  deserialize: (raw: string | null) => T,
): [T, (value: T) => void] => {
  const [value, setValue] = useState<T>(() => deserialize(getStoredValue(key)));

  useEffect(() => {
    setStoredValue(key, value);
  }, [key, value]);

  return [value, setValue];
};
