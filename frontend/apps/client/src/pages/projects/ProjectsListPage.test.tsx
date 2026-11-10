import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import {
  JOB_ID,
  mockDisconnected,
  mockProjectItem,
  mockServerConnection,
  PROJECT_ID,
} from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import {
  mockFailedProject,
  mockProjects,
  mockPulledProject,
} from '@/test/handlers/projects';
import { ProjectsListPage } from './ProjectsListPage';

const C = '/api/v1/client';

// biome-ignore lint/style/useComponentExportOnlyModules: test-only route probe
const Where = () => {
  const loc = useLocation();
  return <div data-testid="location">{loc.pathname + loc.search}</div>;
};

function renderList() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <MemoryRouter initialEntries={['/']}>
          <Routes>
            <Route path="/" element={<ProjectsListPage />} />
            <Route path="*" element={<Where />} />
          </Routes>
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  );
}

const useProjectsList = (projects = mockProjects) =>
  server.use(http.get(`${C}/projects`, () => HttpResponse.json(projects)));

const rowNames = () =>
  screen
    .getAllByRole('row')
    .slice(1)
    .map((r) => within(r).getAllByRole('cell')[0].textContent);

describe('ProjectsListPage', () => {
  it('lists projects with version, environment, size and remotes', async () => {
    useProjectsList();
    renderList();
    expect(
      await screen.findByRole('link', { name: 'Project_1' }),
    ).toHaveAttribute('href', `/projects/${PROJECT_ID}`);
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(
      'Projects',
    );
    expect(
      screen.getByText('Local projects on this machine'),
    ).toBeInTheDocument();

    const installed = screen.getByRole('row', { name: /Project_1/ });
    expect(within(installed).getByText('v7')).toBeInTheDocument();
    expect(
      within(installed).getByRole('button', { name: 'Uninstall' }),
    ).toBeInTheDocument();
    expect(within(installed).getByText('1.2 GB')).toBeInTheDocument();
    expect(
      within(installed).getByRole('button', { name: 'team-nebi' }),
    ).toBeInTheDocument();
    expect(
      within(installed).getByRole('button', { name: 'ghcr.io/example-org' }),
    ).toBeInTheDocument();

    const pulled = screen.getByRole('row', {
      name: /Project_pulled_from_server/,
    });
    expect(within(pulled).getByText('v2')).toBeInTheDocument();
    expect(
      within(pulled).getByRole('button', { name: 'Last installed version' }),
    ).toBeInTheDocument();
    expect(
      within(pulled).getByRole('button', { name: 'Update available' }),
    ).toBeInTheDocument();
    expect(
      within(pulled).getByRole('button', { name: 'Install' }),
    ).toBeInTheDocument();
    expect(within(pulled).getByText('–')).toBeInTheDocument();

    const failed = screen.getByRole('row', { name: /sandbox/ });
    expect(
      within(failed).getByRole('link', { name: /Install failed/ }),
    ).toHaveAttribute('href', `/jobs/${JOB_ID}`);
  });

  it('links "Update available" to the project', async () => {
    useProjectsList();
    renderList();
    await userEvent.click(
      await screen.findByRole('button', { name: 'Update available' }),
    );
    expect(screen.getByTestId('location')).toHaveTextContent(
      `/projects/${mockPulledProject.id}`,
    );
  });

  it('searches and sorts', async () => {
    useProjectsList();
    renderList();
    await screen.findByRole('link', { name: 'Project_1' });
    expect(rowNames()).toEqual([
      'Project_1',
      'Project_pulled_from_server',
      'sandbox',
    ]);

    const nameHeader = screen.getByRole('columnheader', {
      name: /Project name/,
    });
    expect(nameHeader).toHaveAttribute('aria-sort', 'ascending');
    await userEvent.click(
      within(nameHeader).getByRole('button', { name: /Project name/ }),
    );
    expect(nameHeader).toHaveAttribute('aria-sort', 'descending');
    expect(rowNames()[0]).toBe('sandbox');

    await userEvent.click(screen.getByRole('button', { name: /^Size/ }));
    expect(rowNames()[2]).toBe('Project_1');

    await userEvent.type(screen.getByLabelText('Search projects'), 'PULL');
    expect(rowNames()).toEqual(['Project_pulled_from_server']);

    await userEvent.clear(screen.getByLabelText('Search projects'));
    await userEvent.type(screen.getByLabelText('Search projects'), 'zzz');
    expect(screen.getByText('No projects match “zzz”.')).toBeInTheDocument();
  });

  it('installs the last installed version from the list', async () => {
    useProjectsList();
    let installed: string | undefined;
    server.use(
      http.post(`${C}/projects/:id/install`, ({ params }) => {
        installed = params.id as string;
        return HttpResponse.json(
          {
            id: JOB_ID,
            type: 'install_version',
            type_label: 'Install version',
            status: 'completed',
            project_id: params.id,
            project_name: 'Project_pulled_from_server',
            version_number: 2,
            created_at: '2026-09-29T00:00:00Z',
            started_at: null,
            completed_at: null,
            duration_ms: null,
          },
          { status: 202 },
        );
      }),
    );
    renderList();
    const row = await screen.findByRole('row', {
      name: /Project_pulled_from_server/,
    });
    await userEvent.click(within(row).getByRole('button', { name: 'Install' }));
    await waitFor(() => expect(installed).toBe(mockPulledProject.id));
  });

  it('shows installing and uninstalling states', async () => {
    useProjectsList([
      {
        ...mockPulledProject,
        install_status: 'installing',
        active_job_id: JOB_ID,
      },
      {
        ...mockProjectItem,
        install_status: 'uninstalling',
        active_job_id: JOB_ID,
      },
    ]);
    renderList();
    expect(await screen.findByText('Installing…')).toBeInTheDocument();
    expect(screen.getByText('Uninstalling…')).toBeInTheDocument();
  });

  it('deletes a project after confirmation', async () => {
    useProjectsList();
    let deleted: string | undefined;
    server.use(
      http.delete(`${C}/projects/:id`, ({ params }) => {
        deleted = params.id as string;
        return HttpResponse.json(
          {
            id: JOB_ID,
            type: 'delete',
            type_label: 'Delete project',
            status: 'completed',
            project_id: params.id,
            project_name: 'sandbox',
            version_number: null,
            created_at: '2026-09-29T00:00:00Z',
            started_at: null,
            completed_at: null,
            duration_ms: null,
          },
          { status: 202 },
        );
      }),
    );
    renderList();
    await userEvent.click(
      await screen.findByRole('button', { name: 'Actions for sandbox' }),
    );
    await userEvent.click(
      await screen.findByRole('menuitem', { name: /Delete project/ }),
    );
    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByText('Delete sandbox?')).toBeInTheDocument();
    // Not installed: the copy does not mention an environment.
    expect(
      within(dialog).getByText(
        /^The project and its version are removed from this machine\./,
      ),
    ).toBeInTheDocument();
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Delete project' }),
    );
    await waitFor(() => expect(deleted).toBe(mockFailedProject.id));
  });

  it('offers Create new version from the row menu', async () => {
    useProjectsList();
    renderList();
    await userEvent.click(
      await screen.findByRole('button', { name: 'Actions for Project_1' }),
    );
    await userEvent.click(
      await screen.findByRole('menuitem', { name: /Create new version/ }),
    );
    expect(screen.getByTestId('location')).toHaveTextContent(
      `/projects/${PROJECT_ID}/new-version?from=7`,
    );
  });

  it('opens the pixi.toml editor from the New project menu', async () => {
    useProjectsList();
    renderList();
    await userEvent.click(
      await screen.findByRole('button', {
        name: 'More ways to create a project',
      }),
    );
    expect(
      await screen.findByRole('menuitem', { name: /Create from GUI/ }),
    ).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole('menuitem', { name: /Create from pixi.toml/ }),
    );
    expect(screen.getByTestId('location')).toHaveTextContent(
      '/projects/new?mode=toml',
    );
  });

  it('shows the empty state when not connected', async () => {
    useProjectsList([]);
    server.use(
      http.get(`${C}/remote/server`, () => HttpResponse.json(mockDisconnected)),
    );
    renderList();
    expect(await screen.findByText('No projects yet')).toBeInTheDocument();
    expect(
      screen.getByText(
        'Create a project locally, or bring one in from your team’s server or a registry.',
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Connect to server' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Add a registry' }),
    ).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'New project' }));
    expect(screen.getByTestId('location')).toHaveTextContent('/projects/new');
  });

  it('shows the empty state when connected', async () => {
    useProjectsList([]);
    server.use(
      http.get(`${C}/remote/server`, () =>
        HttpResponse.json(mockServerConnection),
      ),
    );
    renderList();
    expect(
      await screen.findByText(
        'Create one locally, or pull an existing project from your server or a registry.',
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Pull from registry' }),
    ).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole('button', { name: 'Pull from server' }),
    );
    expect(screen.getByTestId('location')).toHaveTextContent('/server');
  });

  it('shows an error with a retry', async () => {
    server.use(
      http.get(`${C}/projects`, () =>
        HttpResponse.json({ error: 'database locked' }, { status: 500 }),
      ),
    );
    renderList();
    expect(
      await screen.findByText('Couldn’t load projects'),
    ).toBeInTheDocument();
    expect(screen.getByText('database locked')).toBeInTheDocument();
    useProjectsList();
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(
      await screen.findByRole('link', { name: 'Project_1' }),
    ).toBeInTheDocument();
  });
});
