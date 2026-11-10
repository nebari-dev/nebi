import { Toaster } from '@nebi/ui/components/ui/toast';
import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { ThemeProvider } from '@nebi/ui/hooks/theme-provider';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it } from 'vitest';
import { AppRoutes } from './App';
import { useModeStore } from './store/modeStore';
import {
  JOB_ID,
  mockServerConnection,
  PROJECT_ID,
} from './test/fixtures/contracts';
import { server } from './test/handlers';

function renderAt(path: string) {
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

describe('client routes (issue #649)', () => {
  beforeEach(() => {
    useModeStore.setState({ mode: 'local', loading: false });
  });

  // Every route renders a page with its own <main> and an h1 (units keep
  // both when they replace the placeholders).
  it.each([
    '/',
    '/projects/new',
    `/projects/${PROJECT_ID}`,
    `/projects/${PROJECT_ID}/new-version`,
    `/projects/${PROJECT_ID}/compare`,
    '/server',
    '/registries',
    '/jobs',
    `/jobs/${JOB_ID}`,
  ])('renders %s', async (path) => {
    renderAt(path);
    expect(await screen.findByRole('main')).toBeInTheDocument();
    expect(
      await screen.findByRole('heading', { level: 1 }),
    ).toBeInTheDocument();
  });

  it('redirects /projects to the projects list', async () => {
    renderAt('/projects');
    expect(
      await screen.findByRole('heading', { level: 1, name: 'Projects' }),
    ).toBeInTheDocument();
  });

  it('renders the prototype header', async () => {
    renderAt('/');
    const nav = await screen.findByRole('navigation', { name: 'Primary' });
    for (const label of ['Projects', 'Server', 'Registries', 'Jobs']) {
      expect(nav).toHaveTextContent(label);
    }
    expect(screen.getByRole('link', { name: 'Nebi home' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Theme' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Help/ })).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Notifications' }),
    ).toBeInTheDocument();
  });

  it('shows the connected server badge', async () => {
    server.use(
      http.get('/api/v1/client/remote/server', () =>
        HttpResponse.json(mockServerConnection),
      ),
    );
    renderAt('/');
    expect(
      await screen.findByRole('link', {
        name: `Connected to ${mockServerConnection.url}. Open the Server page`,
      }),
    ).toBeInTheDocument();
  });
});
