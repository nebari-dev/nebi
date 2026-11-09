// The header as seen on the Jobs page: the notifications bell (U8) and the
// Light / Dark / System theme menu from packages/ui (verified here, gap
// "Theme: light / dark / system").
import { Toaster } from '@nebi/ui/components/ui/toast';
import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { ThemeProvider } from '@nebi/ui/hooks/theme-provider';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AppRoutes } from '@/App';
import { useModeStore } from '@/store/modeStore';

function stubColorScheme(prefersDark: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn((query: string) => ({
      matches: prefersDark && query.includes('dark'),
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      onchange: null,
      dispatchEvent: vi.fn(),
    })),
  );
}

function renderApp(path: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <ThemeProvider storageKey="test-theme">
      <QueryClientProvider client={client}>
        <TooltipProvider>
          <Toaster>
            <MemoryRouter initialEntries={[path]}>
              <AppRoutes />
            </MemoryRouter>
          </Toaster>
        </TooltipProvider>
      </QueryClientProvider>
    </ThemeProvider>,
  );
}

beforeEach(() => {
  localStorage.clear();
  document.documentElement.classList.remove('dark');
  useModeStore.setState({ mode: 'local', loading: false });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('header on the Jobs page', () => {
  it('switches the theme between Light, Dark and System', async () => {
    stubColorScheme(true);
    const user = userEvent.setup();
    renderApp('/jobs');
    await screen.findByRole('heading', { level: 1, name: 'Jobs' });
    const html = document.documentElement;

    const pick = async (label: string) => {
      await user.click(screen.getByRole('button', { name: 'Theme' }));
      await user.click(
        await screen.findByRole('menuitemradio', { name: `${label} mode` }),
      );
      await user.keyboard('{Escape}');
    };

    await pick('Light');
    await waitFor(() => expect(html).not.toHaveClass('dark'));
    expect(localStorage.getItem('test-theme')).toBe('light');

    await pick('Dark');
    await waitFor(() => expect(html).toHaveClass('dark'));
    expect(localStorage.getItem('test-theme')).toBe('dark');

    // System follows the OS, which prefers dark here.
    await pick('Light');
    await waitFor(() => expect(html).not.toHaveClass('dark'));
    await pick('System');
    await waitFor(() => expect(html).toHaveClass('dark'));
    expect(localStorage.getItem('test-theme')).toBe('system');
  });

  it('marks Jobs as the current section and shows the bell', async () => {
    renderApp('/jobs');
    await screen.findByRole('heading', { level: 1, name: 'Jobs' });
    expect(
      screen.getByRole('button', { name: /^Notifications/ }),
    ).toBeInTheDocument();
  });
});
