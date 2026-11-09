import { Toaster } from '@nebi/ui/components/ui/toast';
import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TOUR_SEEN_KEY, useTourStore } from '@/components/tour/tourStore';
import { useInstallProgressStore } from '@/hooks/client/detail';
import {
  JOB_ID,
  mockJobDetail,
  mockJobSummary,
  mockProjectItem,
  PROJECT_ID,
} from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import {
  detailRailHandlers,
  MOCK_PIXI_TOML,
  mockDetailVersions,
  mockServerOnlyVersion,
} from '@/test/handlers/detail';
import type { JobDetail, ProjectListItem } from '@/types/contracts';
import { ProjectDetailPage } from './ProjectDetailPage';

const C = '/api/v1/client';

// biome-ignore lint/style/useComponentExportOnlyModules: test-only helper
const LocationProbe = () => {
  const loc = useLocation();
  return (
    <div data-testid="location">
      {loc.pathname}
      {loc.search}
    </div>
  );
};

function renderDetail(path = `/projects/${PROJECT_ID}`) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <Toaster>
          <MemoryRouter initialEntries={[path]}>
            <Routes>
              <Route path="/projects/:id" element={<ProjectDetailPage />} />
              <Route path="*" element={<p>elsewhere</p>} />
            </Routes>
            <LocationProbe />
          </MemoryRouter>
        </Toaster>
      </TooltipProvider>
    </QueryClientProvider>,
  );
}

const useProjectMock = (overrides: Partial<ProjectListItem> = {}) =>
  server.use(
    http.get(`${C}/projects/:id`, () =>
      HttpResponse.json({ ...mockProjectItem, ...overrides }),
    ),
  );

describe('ProjectDetailPage', () => {
  beforeEach(() => {
    window.localStorage.setItem(TOUR_SEEN_KEY, '1');
    useTourStore.getState().reload();
    useInstallProgressStore.setState({ running: {}, done: {}, failed: {} });
    server.use(...detailRailHandlers());
  });
  afterEach(() => {
    window.localStorage.clear();
    vi.restoreAllMocks();
  });

  it('shows the header, the rail grouped by age and the installed version', async () => {
    renderDetail();
    expect(
      await screen.findByRole('heading', { level: 1, name: 'Project_1' }),
    ).toBeInTheDocument();
    expect(screen.getByText('/home/user/Project_1')).toBeInTheDocument();
    const rail = await screen.findByRole('navigation', {
      name: 'Version history',
    });
    for (const label of ['Today', 'This week', 'This month', 'This year']) {
      expect(within(rail).getByText(label)).toBeInTheDocument();
    }
    expect(within(rail).getByText('Newest here')).toBeInTheDocument();
    expect(within(rail).getByText('Newest on server')).toBeInTheDocument();

    const panel = await screen.findByRole('region', { name: 'Version 7' });
    expect(
      within(panel).getByRole('heading', { name: 'Version 7' }),
    ).toBeInTheDocument();
    expect(within(panel).getByText('Installed')).toBeInTheDocument();
    expect(within(panel).getByText(/Sam Garcia \(you\)/)).toBeInTheDocument();
    expect(
      within(panel).getByRole('button', { name: /Uninstall/ }),
    ).toBeInTheDocument();
    expect(within(panel).getByText('1.2 GB on disk')).toBeInTheDocument();
    // Linked project with v7 newer than the server's v6.
    expect(
      within(panel).getByRole('button', { name: /Push to server/ }),
    ).toBeInTheDocument();
    expect(
      await within(panel).findByText('Environment specification'),
    ).toBeInTheDocument();
    expect(
      await within(panel).findByText('conda-forge, defaults'),
    ).toBeInTheDocument();
    expect(within(panel).getByText('Specified (3)')).toBeInTheDocument();
    expect(await within(panel).findByText('3.12.7')).toBeInTheDocument();
  });

  it('selects a version with ?v=N and from the rail', async () => {
    const user = userEvent.setup();
    renderDetail(`/projects/${PROJECT_ID}?v=5`);
    const panel = await screen.findByRole('region', { name: 'Version 5' });
    expect(within(panel).getByText(/Replaces/)).toHaveTextContent(
      'Replaces v7 on disk',
    );
    expect(
      within(panel).getByText('Published to ghcr.io/example-org/project_1:5'),
    ).toBeInTheDocument();

    const rail = screen.getByRole('navigation', { name: 'Version history' });
    await user.click(within(rail).getByText('Bumped Python to 3.12'));
    expect(
      await screen.findByRole('region', { name: 'Version 6' }),
    ).toBeInTheDocument();
    expect(screen.getByTestId('location')).toHaveTextContent('?v=6');
  });

  it('installs a version and shows progress then success', async () => {
    const user = userEvent.setup();
    let status: JobDetail['status'] = 'running';
    let installBody: unknown;
    server.use(
      http.post(`${C}/projects/:id/install`, async ({ request }) => {
        installBody = await request.json();
        return HttpResponse.json(
          { ...mockJobSummary, version_number: 5, status: 'pending' },
          { status: 202 },
        );
      }),
      http.get(`${C}/jobs/:jobId`, () =>
        HttpResponse.json({ ...mockJobDetail, version_number: 5, status }),
      ),
    );
    renderDetail(`/projects/${PROJECT_ID}?v=5`);
    const panel = await screen.findByRole('region', { name: 'Version 5' });
    await user.click(within(panel).getByRole('button', { name: /Install/ }));
    expect(installBody).toEqual({ version: 5 });
    expect(
      await within(panel).findByText(/Installing version 5/),
    ).toBeInTheDocument();
    status = 'completed';
    expect(
      await within(panel).findByText(
        'Version 5 installed · ready to use on this machine',
        {},
        { timeout: 4000 },
      ),
    ).toBeInTheDocument();
    await user.click(within(panel).getByRole('button', { name: 'View log' }));
    expect(screen.getByTestId('location')).toHaveTextContent(`/jobs/${JOB_ID}`);
  });

  it('shows the failure with View job and Try again', async () => {
    const user = userEvent.setup();
    let installs = 0;
    server.use(
      http.post(`${C}/projects/:id/install`, () => {
        installs++;
        return HttpResponse.json(
          { ...mockJobSummary, version_number: 5, status: 'pending' },
          { status: 202 },
        );
      }),
      http.get(`${C}/jobs/:jobId`, () =>
        HttpResponse.json({
          ...mockJobDetail,
          version_number: 5,
          status: 'failed',
          error: 'A package download failed while installing.',
        }),
      ),
    );
    renderDetail(`/projects/${PROJECT_ID}?v=5`);
    const panel = await screen.findByRole('region', { name: 'Version 5' });
    await user.click(within(panel).getByRole('button', { name: /Install/ }));
    const alert = await within(panel).findByRole('alert');
    expect(alert).toHaveTextContent('Version 5 couldn’t install');
    expect(alert).toHaveTextContent(
      'A package download failed while installing.',
    );
    // The alert plus the error toast.
    await waitFor(() =>
      expect(
        screen.getAllByText('Version 5 couldn’t install').length,
      ).toBeGreaterThanOrEqual(2),
    );
    expect(
      within(alert).getByRole('link', { name: 'View job' }),
    ).toHaveAttribute('href', `/jobs/${JOB_ID}`);
    await user.click(within(alert).getByRole('button', { name: /Try again/ }));
    await waitFor(() => expect(installs).toBe(2));
  });

  it('shows a failed install from the project state', async () => {
    useProjectMock({
      failed_install_job_id: JOB_ID,
      install_status: 'install_failed',
    });
    server.use(
      http.get(`${C}/jobs/:jobId`, () =>
        HttpResponse.json({
          ...mockJobDetail,
          version_number: 6,
          status: 'failed',
          error: 'boom',
        }),
      ),
    );
    renderDetail(`/projects/${PROJECT_ID}?v=6`);
    expect(await screen.findByText('Install failed')).toBeInTheDocument();
    const panel = await screen.findByRole('region', { name: 'Version 6' });
    expect(await within(panel).findByRole('alert')).toHaveTextContent(
      'Version 6 couldn’t install',
    );
  });

  it('uninstalls after confirmation', async () => {
    const user = userEvent.setup();
    let uninstalled = false;
    server.use(
      http.post(`${C}/projects/:id/uninstall`, () => {
        uninstalled = true;
        return HttpResponse.json(
          { ...mockJobSummary, type: 'uninstall', status: 'completed' },
          { status: 202 },
        );
      }),
    );
    renderDetail();
    const panel = await screen.findByRole('region', { name: 'Version 7' });
    await user.click(within(panel).getByRole('button', { name: /Uninstall/ }));
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog).toHaveTextContent('Uninstall version 7?');
    expect(dialog).toHaveTextContent('Frees 1.2 GB on this machine.');
    expect(dialog).toHaveTextContent(
      'Nebi remembers version 7 so you can install it again.',
    );
    await user.click(within(dialog).getByRole('button', { name: 'Uninstall' }));
    await waitFor(() => expect(uninstalled).toBe(true));
    expect(
      await screen.findByText('Version 7 uninstalled'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('Freed 1.2 GB on this machine.'),
    ).toBeInTheDocument();
  });

  it('copies the project path', async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });
    renderDetail();
    await user.click(await screen.findByRole('button', { name: 'Copy path' }));
    expect(writeText).toHaveBeenCalledWith('/home/user/Project_1');
    expect(await screen.findByText('Path copied')).toBeInTheDocument();

    writeText.mockRejectedValueOnce(new Error('denied'));
    await user.click(screen.getByRole('button', { name: 'Copy path' }));
    expect(
      (await screen.findAllByText('Couldn’t copy the path')).length,
    ).toBeGreaterThan(0);
  });

  it('searches and filters versions', async () => {
    const user = userEvent.setup();
    renderDetail();
    const rail = await screen.findByRole('navigation', {
      name: 'Version history',
    });
    await within(rail).findByText('Bumped Python to 3.12');
    const search = screen.getByRole('searchbox', { name: 'Search versions' });
    await user.type(search, 'v4');
    expect(
      within(rail).getByText('Added the pytorch channel'),
    ).toBeInTheDocument();
    expect(within(rail).queryByText('Bumped Python to 3.12')).toBeNull();
    await user.clear(search);
    await user.type(search, 'stable');
    expect(
      within(rail).getByText('Swapped tensorflow for scikit-learn'),
    ).toBeInTheDocument();
    expect(within(rail).queryByText('Added the pytorch channel')).toBeNull();
    await user.clear(search);
    await user.type(search, 'zzz');
    expect(
      within(rail).getByText('No versions match “zzz”.'),
    ).toBeInTheDocument();
    await user.clear(search);

    await user.click(screen.getByRole('button', { name: /Filters/ }));
    await user.click(
      await screen.findByRole('menuitemcheckbox', { name: 'Published' }),
    );
    await user.keyboard('{Escape}');
    await waitFor(() =>
      expect(within(rail).queryByText('Bumped Python to 3.12')).toBeNull(),
    );
    expect(
      within(rail).getByText('Swapped tensorflow for scikit-learn'),
    ).toBeInTheDocument();
  });

  it('pushes versions the server lacks', async () => {
    const user = userEvent.setup();
    server.use(
      http.post(`${C}/projects/:id/push`, () =>
        HttpResponse.json({
          pushed_versions: [7],
          server_latest_version: 7,
          server_project_id: 'srv',
        }),
      ),
    );
    renderDetail();
    const panel = await screen.findByRole('region', { name: 'Version 7' });
    await user.click(
      within(panel).getByRole('button', { name: /Push to server/ }),
    );
    expect(await screen.findByText('Version 7 pushed')).toBeInTheDocument();
    expect(
      screen.getByText('This machine and the server are in sync.'),
    ).toBeInTheDocument();
  });

  it('offers Pull for a server-only version', async () => {
    const user = userEvent.setup();
    server.use(
      ...detailRailHandlers([mockServerOnlyVersion, ...mockDetailVersions]),
      http.post(`${C}/projects/:id/versions/:version/pull`, () =>
        HttpResponse.json({ ...mockServerOnlyVersion, server_only: false }),
      ),
    );
    renderDetail(`/projects/${PROJECT_ID}?v=8`);
    const panel = await screen.findByRole('region', { name: 'Version 8' });
    expect(
      within(panel).getByText('On the server · not on this machine yet'),
    ).toBeInTheDocument();
    expect(
      within(panel).getByRole('button', { name: /Create new version/ }),
    ).toBeDisabled();
    expect(
      within(panel).getByRole('button', { name: /Publish/ }),
    ).toBeDisabled();
    await user.click(
      within(panel).getByRole('button', { name: /Pull version 8/ }),
    );
    expect(await screen.findByText('Version 8 pulled')).toBeInTheDocument();
  });

  it('asks before starting a new version from an older one', async () => {
    const user = userEvent.setup();
    renderDetail(`/projects/${PROJECT_ID}?v=5`);
    const panel = await screen.findByRole('region', { name: 'Version 5' });
    await user.click(
      within(panel).getByRole('button', { name: /Create new version/ }),
    );
    const dialog = await screen.findByRole('dialog');
    expect(dialog).toHaveTextContent('Start a new version from version 5?');
    expect(dialog).toHaveTextContent(
      'Version 5 is older than the latest version, v7.',
    );
    expect(dialog).toHaveTextContent('Changes made in versions 6 and 7');
    await user.click(
      within(dialog).getByRole('button', { name: 'Start from version 5' }),
    );
    expect(screen.getByTestId('location')).toHaveTextContent(
      `/projects/${PROJECT_ID}/new-version?from=5`,
    );
  });

  it('goes straight to the new version page from the latest version', async () => {
    const user = userEvent.setup();
    renderDetail();
    const panel = await screen.findByRole('region', { name: 'Version 7' });
    await user.click(
      within(panel).getByRole('button', { name: /Create new version/ }),
    );
    expect(screen.getByTestId('location')).toHaveTextContent(
      `/projects/${PROJECT_ID}/new-version?from=7`,
    );
  });

  it('opens pixi.toml from the Files menu', async () => {
    const user = userEvent.setup();
    renderDetail();
    const panel = await screen.findByRole('region', { name: 'Version 7' });
    await user.click(within(panel).getByRole('button', { name: /Files/ }));
    await user.click(
      await screen.findByRole('menuitem', { name: /View pixi.toml/ }),
    );
    const dialog = await screen.findByRole('dialog');
    expect(dialog).toHaveTextContent('pixi.toml · version 7');
    expect(dialog).toHaveTextContent('/home/user/Project_1/pixi.toml');
    expect(
      await within(dialog).findByText(/name = "Project_1"/),
    ).toBeInTheDocument();
    expect(MOCK_PIXI_TOML).toContain('Project_1');
  });

  it('lists resolved packages per platform with sizes', async () => {
    const user = userEvent.setup();
    renderDetail();
    const panel = await screen.findByRole('region', { name: 'Version 7' });
    await user.click(
      await within(panel).findByRole('tab', { name: 'All Resolved (3)' }),
    );
    expect(await within(panel).findByText('17.5 MB')).toBeInTheDocument();
    expect(
      within(panel).getByRole('combobox', { name: 'Platform' }),
    ).toHaveTextContent('linux-64 · this machine');
    await user.type(
      within(panel).getByRole('searchbox', { name: 'Search packages' }),
      'nothing',
    );
    expect(
      within(panel).getByText('No packages match “nothing”.'),
    ).toBeInTheDocument();
  });

  it('shows Project not found for a missing project', async () => {
    server.use(
      http.get(`${C}/projects/:id`, () =>
        HttpResponse.json(
          { error: 'not found', code: 'not_found' },
          { status: 404 },
        ),
      ),
    );
    renderDetail();
    expect(
      await screen.findByRole('heading', { name: 'Project not found' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: 'Back to projects' }),
    ).toBeInTheDocument();
  });

  it('disables Share for a project that is not on a server', async () => {
    useProjectMock({ remotes: [], server_latest_version: null });
    renderDetail();
    const share = await screen.findByRole('button', { name: /Share/ });
    expect(share).toBeDisabled();
    const panel = await screen.findByRole('region', { name: 'Version 7' });
    expect(
      within(panel).queryByRole('button', { name: /Push to server/ }),
    ).toBeNull();
  });
});
