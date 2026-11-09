import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { JOB_ID, PROJECT_ID } from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import {
  FAILED_JOB_ID,
  QUEUED_JOB_ID,
  RUNNING_JOB_ID,
} from '@/test/handlers/jobs';
import { renderJobsRoute } from './testUtils';

const C = '/api/v1/client';

afterEach(() => {
  vi.restoreAllMocks();
});

describe('JobDetailPage', () => {
  it('shows a failed job with its alert, details and log', async () => {
    renderJobsRoute(`/jobs/${FAILED_JOB_ID}`);
    expect(
      await screen.findByRole('heading', {
        level: 1,
        name: 'Install version 2',
      }),
    ).toBeInTheDocument();
    expect(screen.getByText('Failed')).toBeInTheDocument();
    expect(screen.getByText('Job failed')).toBeInTheDocument();
    expect(
      screen.getByText(
        'This job couldn’t be completed successfully. Please see logs for details.',
      ),
    ).toBeInTheDocument();
    const nav = screen.getByRole('navigation', { name: 'breadcrumb' });
    expect(within(nav).getByRole('link', { name: 'Jobs' })).toHaveAttribute(
      'href',
      '/jobs',
    );
    for (const label of [
      'Project',
      'Version',
      'Started',
      'Ended',
      'Duration',
    ]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
    expect(screen.getByText('42s')).toBeInTheDocument();
    // The project was deleted: name without a link, View project disabled.
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'View project' }),
      ).toBeDisabled(),
    );
    expect(
      screen.queryByRole('link', { name: 'sandbox' }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole('log', { name: 'Install version log' }),
    ).toHaveTextContent('Error: failed to fetch tensorflow');
    expect(
      screen.queryByRole('button', { name: 'Cancel job' }),
    ).not.toBeInTheDocument();
  });

  it('searches the log with a match count and highlights matches', async () => {
    const user = userEvent.setup();
    renderJobsRoute(`/jobs/${FAILED_JOB_ID}`);
    await screen.findByRole('log');
    const search = screen.getByRole('searchbox', { name: 'Search logs' });
    await user.type(search, 'ERROR');
    expect(screen.getByText('1 matching line')).toBeInTheDocument();
    expect(screen.getByRole('log').querySelectorAll('mark')).toHaveLength(1);
    await user.clear(search);
    await user.type(search, 'i');
    expect(screen.getByText('3 matching lines')).toBeInTheDocument();
    await user.clear(search);
    await user.type(search, 'nope');
    expect(screen.getByText('No matching lines')).toBeInTheDocument();
  });

  it('copies and downloads the log', async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });
    const createObjectURL = vi.fn(() => 'blob:log');
    const revokeObjectURL = vi.fn();
    Object.assign(URL, { createObjectURL, revokeObjectURL });
    const clicked: string[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      clicked.push(this.download);
    });

    renderJobsRoute(`/jobs/${FAILED_JOB_ID}`);
    await screen.findByRole('log');
    await user.click(screen.getByRole('button', { name: 'Copy' }));
    expect(writeText).toHaveBeenCalledWith(
      'Running: pixi install -v\nError: failed to fetch tensorflow\nCleaning up',
    );
    expect((await screen.findAllByText('Log copied')).length).toBeGreaterThan(
      0,
    );

    await user.click(screen.getByRole('button', { name: 'Download' }));
    expect(clicked).toEqual(['nebi-install_version-44444444.log']);
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:log');

    writeText.mockRejectedValueOnce(new Error('denied'));
    await user.click(screen.getByRole('button', { name: 'Copy' }));
    expect(
      (await screen.findAllByText('Couldn’t copy the log')).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText('Select the text in the log and copy it instead.')
        .length,
    ).toBeGreaterThan(0);
  });

  it('streams a running job and cancels it', async () => {
    const user = userEvent.setup();
    let cancelled = false;
    server.use(
      http.post(`${C}/jobs/:jobId/cancel`, ({ params }) => {
        cancelled = params.jobId === RUNNING_JOB_ID;
        return HttpResponse.json({
          id: RUNNING_JOB_ID,
          type: 'create_version',
          type_label: 'Create version',
          status: 'cancelled',
          project_id: PROJECT_ID,
          project_name: 'Project_1',
          version_number: null,
          created_at: new Date().toISOString(),
          started_at: new Date().toISOString(),
          completed_at: new Date().toISOString(),
          duration_ms: 1000,
        });
      }),
    );
    renderJobsRoute(`/jobs/${RUNNING_JOB_ID}`);
    expect(
      await screen.findByRole('heading', { level: 1, name: 'Create version' }),
    ).toBeInTheDocument();
    expect(screen.getByText('Running')).toBeInTheDocument();
    expect(
      screen.getByText('Streaming live while the job runs'),
    ).toBeInTheDocument();
    expect(screen.getByRole('log')).toHaveTextContent('Solving environment');
    expect(screen.getAllByText('-').length).toBeGreaterThan(0); // Ended

    await user.click(screen.getByRole('button', { name: 'Cancel job' }));
    expect(
      await screen.findByText('Create version cancelled'),
    ).toBeInTheDocument();
    expect(cancelled).toBe(true);
  });

  it('shows the waiting state for a queued job', async () => {
    renderJobsRoute(`/jobs/${QUEUED_JOB_ID}`);
    expect(
      await screen.findByText(
        'Waiting to start. The log appears once the job runs.',
      ),
    ).toBeInTheDocument();
    expect(screen.getByText('Queued')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copy' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Download' })).toBeDisabled();
  });

  it('links a finished job back to its project', async () => {
    const user = userEvent.setup();
    renderJobsRoute(`/jobs/${JOB_ID}`);
    await screen.findByRole('heading', { level: 1, name: 'Install version 7' });
    expect(screen.getByRole('link', { name: 'Project_1' })).toHaveAttribute(
      'href',
      `/projects/${PROJECT_ID}`,
    );
    await user.click(screen.getByRole('button', { name: 'View project' }));
    expect(screen.getByTestId('location')).toHaveTextContent(
      `/projects/${PROJECT_ID}`,
    );
  });

  it('shows Job not found for an unknown job', async () => {
    server.use(
      http.get(`${C}/jobs/:jobId`, () =>
        HttpResponse.json(
          { error: 'not found', code: 'not_found' },
          { status: 404 },
        ),
      ),
    );
    renderJobsRoute('/jobs/77777777-7777-4777-8777-777777777777');
    expect(
      await screen.findByRole('heading', { level: 1, name: 'Job not found' }),
    ).toBeInTheDocument();
    expect(
      screen.getByText('It may have been cleared from this machine.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Back to Jobs' })).toHaveAttribute(
      'href',
      '/jobs',
    );
  });
});
