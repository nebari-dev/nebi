import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { NEBI_JOB_COMPLETED_MESSAGE } from '@/lib/hostBridge';
import { useModeStore } from '@/store/modeStore';
import { mockJob, server } from '@/test/handlers';
import type { Job } from '@/types';
import { useHostJobNotifications } from './useHostJobNotifications';

function job(overrides: Partial<Job>): Job {
  return {
    ...mockJob,
    type: 'env_uninstall',
    status: 'running',
    ...overrides,
  };
}

function embedInParent() {
  const parent = { postMessage: vi.fn() };
  vi.stubGlobal('parent', parent);
  return parent;
}

function renderNotifications() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );

  const hook = renderHook(() => useHostJobNotifications(), { wrapper });

  return { ...hook, queryClient };
}

async function refreshJobs(queryClient: QueryClient) {
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: ['jobs'] });
  });
}

beforeEach(() => {
  useModeStore.setState({ mode: 'local', features: {}, loading: false });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('useHostJobNotifications', () => {
  it('posts once when any job type completes in an embedded local app', async () => {
    const parent = embedInParent();
    let status: Job['status'] = 'running';

    server.use(
      http.get('/api/v1/jobs', () => HttpResponse.json([job({ status })])),
    );

    const { queryClient } = renderNotifications();

    await waitFor(() =>
      expect(queryClient.getQueryData<Job[]>(['jobs'])?.[0].status).toBe(
        'running',
      ),
    );

    status = 'completed';
    await refreshJobs(queryClient);

    await waitFor(() => expect(parent.postMessage).toHaveBeenCalledTimes(1));
    expect(parent.postMessage).toHaveBeenCalledWith(
      {
        type: NEBI_JOB_COMPLETED_MESSAGE,
        jobType: 'env_uninstall',
        status: 'completed',
        workspaceId: 'ws-1',
      },
      window.location.origin,
    );

    await refreshJobs(queryClient);

    expect(parent.postMessage).toHaveBeenCalledTimes(1);
  });

  it('does not post outside an embedded host', async () => {
    const postMessage = vi.spyOn(window, 'postMessage');
    let status: Job['status'] = 'running';

    server.use(
      http.get('/api/v1/jobs', () => HttpResponse.json([job({ status })])),
    );

    const { queryClient } = renderNotifications();

    status = 'completed';
    await refreshJobs(queryClient);

    expect(postMessage).not.toHaveBeenCalled();
    postMessage.mockRestore();
  });

  it('does not post when a job fails', async () => {
    const parent = embedInParent();
    let status: Job['status'] = 'running';

    server.use(
      http.get('/api/v1/jobs', () => HttpResponse.json([job({ status })])),
    );

    const { queryClient } = renderNotifications();

    await waitFor(() =>
      expect(queryClient.getQueryData<Job[]>(['jobs'])?.[0].status).toBe(
        'running',
      ),
    );

    status = 'failed';
    await refreshJobs(queryClient);

    expect(parent.postMessage).not.toHaveBeenCalled();
  });

  it('uses the first successful fetch only to initialize state', async () => {
    const parent = embedInParent();

    server.use(
      http.get('/api/v1/jobs', () =>
        HttpResponse.json([job({ status: 'completed' })]),
      ),
    );

    const { queryClient } = renderNotifications();

    await waitFor(() =>
      expect(queryClient.getQueryData<Job[]>(['jobs'])?.[0].status).toBe(
        'completed',
      ),
    );

    expect(parent.postMessage).not.toHaveBeenCalled();
  });

  it('posts for a completed job first observed after initialization', async () => {
    const parent = embedInParent();
    let jobs = [job({ id: 'job-a', status: 'running' })];

    server.use(http.get('/api/v1/jobs', () => HttpResponse.json(jobs)));

    const { queryClient } = renderNotifications();

    await waitFor(() =>
      expect(queryClient.getQueryData<Job[]>(['jobs'])?.[0].id).toBe('job-a'),
    );

    jobs = [
      job({ id: 'job-a', status: 'running' }),
      job({
        id: 'job-b',
        workspace_id: 'ws-2',
        type: 'delete',
        status: 'completed',
      }),
    ];
    await refreshJobs(queryClient);

    await waitFor(() => expect(parent.postMessage).toHaveBeenCalledTimes(1));
    expect(parent.postMessage).toHaveBeenCalledWith(
      {
        type: NEBI_JOB_COMPLETED_MESSAGE,
        jobType: 'delete',
        status: 'completed',
        workspaceId: 'ws-2',
      },
      window.location.origin,
    );
  });
});
