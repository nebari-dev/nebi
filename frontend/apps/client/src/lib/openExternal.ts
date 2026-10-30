declare global {
  interface Window {
    runtime?: {
      BrowserOpenURL: (url: string) => void;
    };
  }
}

export function openExternal(url: string): void {
  if (window.runtime?.BrowserOpenURL) {
    window.runtime.BrowserOpenURL(url);
  } else {
    window.open(url, '_blank', 'noopener,noreferrer');
  }
}
