// Owner: U5 (remote). Small wording helpers shared by the server screens.

/** "https://nebi.example.com/" -> "nebi.example.com" (prototype display). */
export const displayServerUrl = (url: string) =>
  url.replace(/^https?:\/\//i, '').replace(/\/+$/, '');

/** Prototype "Last synced" wording: Just now / 5 minutes ago / 2 hours ago. */
export function syncedAgo(iso: string | null | undefined, now = Date.now()) {
  if (!iso) return 'Never';
  const at = Date.parse(iso);
  if (Number.isNaN(at)) return 'Never';
  const minutes = Math.floor(Math.max(0, now - at) / 60_000);
  if (minutes < 1) return 'Just now';
  if (minutes < 60) return `${minutes} minute${minutes === 1 ? '' : 's'} ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours} hour${hours === 1 ? '' : 's'} ago`;
  const days = Math.floor(hours / 24);
  return `${days} days ago`;
}

/** Prototype wording: "version 3" or "versions 3, 4 and 5". */
export function versionList(numbers: number[]) {
  if (numbers.length === 1) return `version ${numbers[0]}`;
  return `versions ${numbers.slice(0, -1).join(', ')} and ${numbers.at(-1)}`;
}

export const capitalize = (s: string) =>
  s.length === 0 ? s : s.charAt(0).toUpperCase() + s.slice(1);

/** The prototype's URL check, applied without the scheme. */
const HOST_PATTERN = /^[a-z0-9.-]+(:\d+)?(\/.*)?$/i;

/**
 * Client-side check of the Server URL field (the backend checks again).
 * Returns the prototype error copy, or undefined when the URL looks valid.
 */
export function serverUrlError(raw: string): string | undefined {
  const value = raw
    .trim()
    .replace(/^https?:\/\//i, '')
    .replace(/\/$/, '');
  if (!value) return 'Enter the server’s URL.';
  if (!HOST_PATTERN.test(value))
    return 'That doesn’t look like a URL. Try nebi.example.com.';
  return undefined;
}
