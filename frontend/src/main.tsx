import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import '@fontsource/ibm-plex-sans/400.css';
import '@fontsource/ibm-plex-sans/500.css';
import '@fontsource/ibm-plex-sans/600.css';
import '@fontsource/ibm-plex-sans/700.css';
import '@fontsource/fira-code/400.css';
import '@fontsource/fira-code/500.css';
import './index.css';
import App from './App.tsx';
import { ThemeProvider } from './hooks/theme-provider';
import { loadBrandingConfig } from './lib/brandingConfig';
import { THEME_STORAGE_KEY } from './lib/theme';

const rootElement = document.getElementById('root');

if (!rootElement) {
  throw new Error('Root element #root was not found');
}

async function bootstrap(root: HTMLElement): Promise<void> {
  await loadBrandingConfig();

  createRoot(root).render(
    <StrictMode>
      <ThemeProvider storageKey={THEME_STORAGE_KEY}>
        <App />
      </ThemeProvider>
    </StrictMode>,
  );
}

void bootstrap(rootElement);
