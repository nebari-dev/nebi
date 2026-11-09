import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { describe, expect, it } from 'vitest';
import { PROJECT_ID } from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import { FAILED_JOB_ID, RUNNING_JOB_ID } from '@/test/handlers/jobs';
import { renderJobsRoute } from './testUtils';

const C = '/api/v1/client';

const jobRows = () =>
  within(screen.getByRole('table', { name: 'Jobs' }))
    .getAllByRole('row')
    .slice(1);

describe('JobsPage', () => {
  it('lists jobs on this machine with the prototype columns', async () => {
    renderJobsRoute('/jobs');
    expect(
      screen.getByRole('heading', { level: 1, name: 'Jobs' }),
    ).toBeInTheDocument();
    expect(
      screen.getByText('Background tasks running on this machine'),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole('link', { name: 'Install version 2' }),
    ).toBeInTheDocument();

    const rows = jobRows();
    expect(rows).toHaveLength(4);
    // Newest start first: queued (by creation time), running, failed, done.
    expect(rows[0]).toHaveTextContent('Uninstall');
    expect(rows[0]).toHaveTextContent('Queued');
    expect(rows[0]).toHaveTextContent('Just now');
    expect(rows[1]).toHaveTextContent('Running');
    expect(rows[1]).toHaveTextContent('2 min ago');
    expect(rows[2]).toHaveTextContent('Failed');
    expect(rows[2]).toHaveTextContent('42s');
    expect(rows[3]).toHaveTextContent('Install version 7');
    expect(rows[3]).toHaveTextContent('Succeeded');
    expect(rows[3]).toHaveTextContent('30s');

    // A project that still exists links to it; a deleted one does not.
    expect(
      within(rows[1]).getByRole('link', { name: 'Project_1' }),
    ).toHaveAttribute('href', `/projects/${PROJECT_ID}`);
    await waitFor(() =>
      expect(
        within(rows[2]).queryByRole('link', { name: 'sandbox' }),
      ).not.toBeInTheDocument(),
    );
    expect(rows[2]).toHaveTextContent('sandbox');
  });

  it('searches by job and project and offers Clear filters', async () => {
    const user = userEvent.setup();
    renderJobsRoute('/jobs');
    await screen.findByRole('link', { name: 'Install version 2' });

    await user.type(
      screen.getByRole('searchbox', { name: 'Search jobs' }),
      'sandbox',
    );
    expect(jobRows()).toHaveLength(1);
    expect(jobRows()[0]).toHaveTextContent('Install version 2');

    await user.clear(screen.getByRole('searchbox', { name: 'Search jobs' }));
    await user.type(
      screen.getByRole('searchbox', { name: 'Search jobs' }),
      'nothing like this',
    );
    expect(
      screen.getByText(/No jobs match these filters\./),
    ).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Clear filters' }));
    expect(jobRows()).toHaveLength(4);
  });

  it('filters by status through the API and keeps the filter in the URL', async () => {
    const user = userEvent.setup();
    renderJobsRoute('/jobs');
    await screen.findByRole('link', { name: 'Install version 2' });

    await user.click(
      screen.getByRole('button', { name: 'Status: All. Change filter' }),
    );
    await user.click(
      await screen.findByRole('menuitemradio', { name: 'Failed' }),
    );
    await waitFor(() => expect(jobRows()).toHaveLength(1));
    expect(jobRows()[0]).toHaveTextContent('Failed');
    expect(
      screen.getByRole('button', { name: 'Status: Failed. Change filter' }),
    ).toBeInTheDocument();
    expect(screen.getByTestId('location')).toHaveTextContent(
      '/jobs?status=failed',
    );
  });

  it('applies type and project filters from the URL', async () => {
    renderJobsRoute(`/jobs?type=create_version&project=${PROJECT_ID}`);
    expect(
      await screen.findByRole('link', { name: 'Create version' }),
    ).toBeInTheDocument();
    expect(jobRows()).toHaveLength(1);
    expect(
      screen.getByRole('button', {
        name: 'Job type: Create version. Change filter',
      }),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole('button', {
        name: 'Project: Project_1. Change filter',
      }),
    ).toBeInTheDocument();
  });

  it('searches projects in the Project filter', async () => {
    const user = userEvent.setup();
    renderJobsRoute('/jobs');
    await screen.findByRole('link', { name: 'Install version 2' });
    await user.click(
      screen.getByRole('button', { name: 'Project: All. Change filter' }),
    );
    const search = await screen.findByRole('textbox', {
      name: 'Search projects',
    });
    expect(
      screen.getByRole('menuitemradio', { name: 'Project_1' }),
    ).toBeInTheDocument();
    await user.type(search, 'zzz');
    expect(
      screen.queryByRole('menuitemradio', { name: 'Project_1' }),
    ).not.toBeInTheDocument();
    expect(screen.getByText('No match')).toBeInTheDocument();
  });

  it('sorts by Started', async () => {
    const user = userEvent.setup();
    renderJobsRoute('/jobs');
    await screen.findByRole('link', { name: 'Install version 2' });
    const started = screen.getByRole('columnheader', { name: /Started/ });
    expect(started).toHaveAttribute('aria-sort', 'descending');
    await user.click(within(started).getByRole('button'));
    await waitFor(() =>
      expect(jobRows()[0]).toHaveTextContent('Install version 7'),
    );
    expect(
      screen.getByRole('columnheader', { name: /Started/ }),
    ).toHaveAttribute('aria-sort', 'ascending');
  });

  it('row menu: View project and Cancel job on a running job', async () => {
    const user = userEvent.setup();
    renderJobsRoute('/jobs');
    await screen.findByRole('link', { name: 'Install version 2' });

    await user.click(
      screen.getByRole('button', {
        name: 'Actions for Create version on Project_1',
      }),
    );
    expect(
      await screen.findByRole('menuitem', { name: 'View project' }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole('menuitem', { name: 'Cancel job' }));
    expect(
      await screen.findByText('Create version cancelled'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('Nothing changed in Project_1.'),
    ).toBeInTheDocument();

    await user.click(
      screen.getByRole('button', {
        name: 'Actions for Create version on Project_1',
      }),
    );
    await user.click(
      await screen.findByRole('menuitem', { name: 'View project' }),
    );
    expect(screen.getByTestId('location')).toHaveTextContent(
      `/projects/${PROJECT_ID}`,
    );
  });

  it('row menu: View project is disabled when the project is gone', async () => {
    const user = userEvent.setup();
    renderJobsRoute('/jobs');
    await screen.findByRole('link', { name: 'Install version 2' });
    await user.click(
      screen.getByRole('button', {
        name: 'Actions for Install version on sandbox',
      }),
    );
    const view = await screen.findByRole('menuitem', { name: 'View project' });
    await waitFor(() => expect(view).toHaveAttribute('aria-disabled', 'true'));
    expect(
      screen.queryByRole('menuitem', { name: 'Cancel job' }),
    ).not.toBeInTheDocument();
  });

  it('opens a job when its row is clicked', async () => {
    const user = userEvent.setup();
    renderJobsRoute('/jobs');
    await screen.findByRole('link', { name: 'Install version 2' });
    await user.click(jobRows()[1].querySelectorAll('td')[2]);
    expect(screen.getByTestId('location')).toHaveTextContent(
      `/jobs/${RUNNING_JOB_ID}`,
    );
  });

  it('shows the empty state and Refresh refetches', async () => {
    const user = userEvent.setup();
    let calls = 0;
    server.use(
      http.get(`${C}/jobs`, () => {
        calls++;
        return HttpResponse.json([]);
      }),
    );
    renderJobsRoute('/jobs');
    expect(
      await screen.findByText('No jobs have run on this machine yet.'),
    ).toBeInTheDocument();
    const before = calls;
    await user.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(calls).toBeGreaterThan(before));
  });

  it('links failed jobs to their page', async () => {
    renderJobsRoute('/jobs?status=failed');
    expect(
      await screen.findByRole('link', { name: 'Install version 2' }),
    ).toHaveAttribute('href', `/jobs/${FAILED_JOB_ID}`);
  });
});
