// Base path for the application, injected at runtime by the backend.
// Falls back to empty string (root path) when not set.
declare global {
  interface Window {
    __NEBI_BASE_PATH__?: string;
  }
}

export function getBasePath(): string {
  return window.__NEBI_BASE_PATH__ || '';
}

export function getApiBaseUrl(): string {
  return `${getBasePath()}/api/v1`;
}
