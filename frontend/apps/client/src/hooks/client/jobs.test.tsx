import { Toaster } from '@nebi/ui/components/ui/toast';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  act,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { mockJobSummary, PROJECT_ID } from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import { RUNNING_JOB_ID } from '@/test/handlers/jobs';
import type { JobSummary } from '@/types/contracts';
import { trackJobToast, useJobLogStream, useJobs } from './jobs';

const C = '/api/v1/client';

const running: JobSummary = {
  ...mockJobSummary,
  id: '88888888-8888-4888-8888-888888888888',
  type: 'install_version',
  type_label: 'Install version',
  status: 'running',
  completed_at: null,
  duration_ms: null,
};

/** Serves the tracked job: running for `runningPolls` polls, then `final`. */
function serveJob(final: Partial<JobSummary>, runningPolls = 1) {
  let polls = 0;
  server.use(
    http.get(`${C}/jobs/:jobId`, () => {
      polls++;
      return HttpResponse.json(
        polls <= runningPolls
          ? { ...running, logs: '' }
          : { ...running, ...final, logs: '' },
      );
    }),
  );
}

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe('trackJobToast', () => {
  it('follows a job to success', async () => {
    render(<Toaster />);
    serveJob({ status: 'completed', version_number: 5 });
    const onSettled = vi.fn();
    trackJobToast(running, {
      title: 'Installing version 5',
      description: 'Install job started',
      successTitle: (j) => `Version ${j.version_number} installed`,
      successDescription: 'Ready to use on this machine.',
      intervalMs: 5,
      onSettled,
    });
    expect(
      (await screen.findAllByText('Installing version 5')).length,
    ).toBeGreaterThan(0);
    expect(
      (await screen.findAllByText('Version 5 installed')).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText('Ready to use on this machine.').length,
    ).toBeGreaterThan(0);
    expect(onSettled).toHaveBeenCalledWith(
      expect.objectContaining({ status: 'completed' }),
    );
  });

  it('offers View job on failure', async () => {
    render(<Toaster />);
    serveJob({ status: 'failed', error: 'pixi exploded' });
    const onViewJob = vi.fn();
    trackJobToast(
      { ...running, id: '99999999-9999-4999-8999-999999999999' },
      {
        title: 'Installing version 5',
        errorTitle: 'Version 5 couldn’t install',
        intervalMs: 5,
        onViewJob,
      },
    );
    expect(
      (await screen.findAllByText('Version 5 couldn’t install')).length,
    ).toBeGreaterThan(0);
    expect(screen.getAllByText('pixi exploded').length).toBeGreaterThan(0);
    // The toast region is aria-hidden in jsdom while a toast animates in.
    await userEvent.click(
      screen.getByRole('button', { name: 'View job', hidden: true }),
    );
    expect(onViewJob).toHaveBeenCalled();
  });

  it('cancels from the toast and reports nothing changed', async () => {
    render(<Toaster />);
    let cancelled = false;
    server.use(
      http.post(`${C}/jobs/:jobId/cancel`, () => {
        cancelled = true;
        return HttpResponse.json({ ...running, status: 'cancelled' });
      }),
      http.get(`${C}/jobs/:jobId`, () =>
        HttpResponse.json({
          ...running,
          status: cancelled ? 'cancelled' : 'running',
          logs: '',
        }),
      ),
    );
    trackJobToast(
      { ...running, id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' },
      { title: 'Pulling ml-baseline', cancellable: true, intervalMs: 20 },
    );
    await userEvent.click(
      await screen.findByRole('button', { name: 'Cancel' }),
    );
    expect(
      (await screen.findAllByText('Pulling ml-baseline cancelled')).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText('Nothing changed in Project_1.').length,
    ).toBeGreaterThan(0);
    expect(cancelled).toBe(true);
  });

  it('keeps one toast per job', () => {
    const job = { ...running, id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb' };
    const stop = trackJobToast(job, { intervalMs: 10_000 });
    expect(trackJobToast(job)).toBe(stop);
    stop();
  });
});

describe('useJobLogStream', () => {
  it('collects the streamed log of an active job', async () => {
    const { result } = renderHook(() => useJobLogStream(RUNNING_JOB_ID, true), {
      wrapper,
    });
    await waitFor(() =>
      expect(result.current.logs).toBe('Solving environment\n'),
    );
    await waitFor(() => expect(result.current.streaming).toBe(false));
  });

  it('joins chunks exactly as written', async () => {
    server.use(
      http.get(`${C}/jobs/:jobId/logs/stream`, () => {
        const encoder = new TextEncoder();
        const stream = new ReadableStream({
          start(controller) {
            controller.enqueue(encoder.encode('data: one\ndata: \n\n'));
            controller.enqueue(encoder.encode('data: tw'));
            controller.enqueue(encoder.encode('o (partial'));
            controller.enqueue(encoder.encode(')\n\ndata: \ndata: three\n\n'));
            controller.enqueue(
              encoder.encode('event: done\ndata: completed\n\n'),
            );
            controller.close();
          },
        });
        return new HttpResponse(stream, {
          headers: { 'Content-Type': 'text/event-stream' },
        });
      }),
    );
    const { result } = renderHook(() => useJobLogStream(PROJECT_ID, true), {
      wrapper,
    });
    await waitFor(() =>
      expect(result.current.logs).toBe('one\ntwo (partial)\nthree'),
    );
  });

  it('does nothing for a finished job', () => {
    const { result } = renderHook(
      () => useJobLogStream(RUNNING_JOB_ID, false),
      {
        wrapper,
      },
    );
    expect(result.current).toEqual({ logs: null, streaming: false });
  });
});

describe('useJobs', () => {
  it('passes filters to the API', async () => {
    const { result } = renderHook(
      () => useJobs({ status: ['failed'], sort: 'started_at' }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.data).toHaveLength(1));
    expect(result.current.data?.[0].status).toBe('failed');
    await act(async () => {});
  });
});
