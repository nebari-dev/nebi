// Owner: U8 (jobs). Renders the jobs pages with the providers they need
// (query client, toasts, router with the job/project routes).
import { Toaster } from '@nebi/ui/components/ui/toast';
import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { ThemeProvider } from '@nebi/ui/hooks/theme-provider';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render } from '@testing-library/react';
import type { ReactElement } from 'react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { JobDetailPage } from './JobDetailPage';
import { JobsPage } from './JobsPage';

// biome-ignore lint/style/useComponentExportOnlyModules: test-only helper module, never hot reloaded.
const Where = () => {
  const { pathname, search } = useLocation();
  return <output data-testid="location">{pathname + search}</output>;
};

export function renderJobsRoute(path: string, extra?: ReactElement) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });
  return render(
    <ThemeProvider storageKey="test-theme">
      <QueryClientProvider client={client}>
        <TooltipProvider>
          <Toaster>
            <MemoryRouter initialEntries={[path]}>
              {extra}
              <Routes>
                <Route path="/jobs" element={<JobsPage />} />
                <Route path="/jobs/:id" element={<JobDetailPage />} />
                <Route path="/projects/:id" element={<h1>Project page</h1>} />
              </Routes>
              <Where />
            </MemoryRouter>
          </Toaster>
        </TooltipProvider>
      </QueryClientProvider>
    </ThemeProvider>,
  );
}
