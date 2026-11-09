import { Toaster } from '@nebi/ui/components/ui/toast';
import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import type { ReactNode } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { ConnectedServerBadge } from '@/components/layout/ConnectedServerBadge';
import {
  mockProjectItem,
  mockServerConnection,
  mockVersionItem,
  PROJECT_ID,
} from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import { mockPullJob } from '@/test/handlers/remote';
import type { ProjectListItem, VersionItem } from '@/types/contracts';
import {
  capitalize,
  displayServerUrl,
  serverUrlError,
  syncedAgo,
  versionList,
} from './format';
import { PullVersionButton } from './PullVersionButton';
import { PushToServerButton } from './PushToServerButton';
import { trackPullToast } from './pullToast';
import { canPush, versionsToPush } from './pushPlan';
import { SyncMarkerLabel } from './SyncMarkerLabel';

const C = '/api/v1/client';

function wrap(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <Toaster>
          <MemoryRouter>{ui}</MemoryRouter>
        </Toaster>
      </TooltipProvider>
    </QueryClientProvider>,
  );
}

const connected = () =>
  server.use(
    http.get(`${C}/remote/server`, () =>
      HttpResponse.json(mockServerConnection),
    ),
  );

const v = (n: number, extra: Partial<VersionItem> = {}): VersionItem => ({
  ...mockVersionItem,
  number: n,
  tags: [],
  sync: null,
  installed: false,
  ...extra,
});

describe('format helpers', () => {
  it('formats the prototype wording', () => {
    expect(displayServerUrl('https://nebi.example.com/')).toBe(
      'nebi.example.com',
    );
    expect(versionList([3])).toBe('version 3');
    expect(versionList([3, 4, 5])).toBe('versions 3, 4 and 5');
    expect(capitalize('versions 6 and 7')).toBe('Versions 6 and 7');
    const now = Date.parse('2026-11-09T12:00:00Z');
    expect(syncedAgo('2026-11-09T11:59:40Z', now)).toBe('Just now');
    expect(syncedAgo('2026-11-09T11:59:00Z', now)).toBe('1 minute ago');
    expect(syncedAgo('2026-11-09T10:00:00Z', now)).toBe('2 hours ago');
    expect(syncedAgo(null, now)).toBe('Never');
    expect(serverUrlError('')).toBe('Enter the server’s URL.');
    expect(serverUrlError('nebi example')).toBe(
      'That doesn’t look like a URL. Try nebi.example.com.',
    );
    expect(serverUrlError('http://localhost:8460/')).toBeUndefined();
  });
});

describe('push plan', () => {
  const linked: ProjectListItem = {
    ...mockProjectItem,
    server_latest_version: 5,
  };
  const versions = [v(7), v(6), v(5), v(4)];

  it('offers push only above the server’s newest version', () => {
    expect(canPush(linked, v(6), true)).toBe(true);
    expect(canPush(linked, v(5), true)).toBe(false);
    expect(canPush(linked, v(6, { server_only: true }), true)).toBe(false);
    expect(canPush(linked, v(6), false)).toBe(false);
    expect(
      canPush({ ...linked, server_latest_version: null }, v(6), true),
    ).toBe(false);
    expect(versionsToPush(linked, versions, 7)).toEqual([6, 7]);
  });

  it('offers the first push on the newest version of an unlinked project', () => {
    const unlinked = {
      ...mockProjectItem,
      remotes: [],
      server_latest_version: null,
      latest_version: 7,
    };
    expect(canPush(unlinked, v(7), true)).toBe(true);
    expect(canPush(unlinked, v(6), true)).toBe(false);
  });
});

describe('PushToServerButton', () => {
  it('pushes up to the version and reports In sync', async () => {
    const user = userEvent.setup();
    connected();
    let body: unknown;
    server.use(
      http.post(`${C}/projects/:id/push`, async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({
          pushed_versions: [6, 7],
          server_latest_version: 7,
          server_project_id: 'x',
        });
      }),
    );
    wrap(
      <PushToServerButton
        project={{ ...mockProjectItem, server_latest_version: 5 }}
        version={v(7)}
        versions={[v(7), v(6), v(5)]}
      />,
    );
    await user.click(
      await screen.findByRole('button', { name: 'Push to server' }),
    );
    expect(
      await screen.findByText('Versions 6 and 7 pushed'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('This machine and the server are in sync.'),
    ).toBeInTheDocument();
    expect(body).toEqual({ up_to: 7 });
  });

  it('says which versions stay only on this machine', async () => {
    const user = userEvent.setup();
    connected();
    server.use(
      http.post(`${C}/projects/:id/push`, () =>
        HttpResponse.json({
          pushed_versions: [6],
          server_latest_version: 6,
          server_project_id: 'x',
        }),
      ),
    );
    wrap(
      <PushToServerButton
        project={{ ...mockProjectItem, server_latest_version: 5 }}
        version={v(6)}
        versions={[v(7), v(6), v(5)]}
      />,
    );
    await user.click(
      await screen.findByRole('button', { name: 'Push to server' }),
    );
    expect(await screen.findByText('Version 6 pushed')).toBeInTheDocument();
    expect(
      screen.getByText('Version 7 is still only on this machine.'),
    ).toBeInTheDocument();
  });

  it('renders nothing for a version already on the server', async () => {
    connected();
    const { container } = wrap(
      <PushToServerButton
        project={{ ...mockProjectItem, server_latest_version: 7 }}
        version={v(7)}
      />,
    );
    await waitFor(() => expect(container.querySelector('button')).toBeNull());
  });
});

describe('PullVersionButton', () => {
  it('pulls a server-only version', async () => {
    const user = userEvent.setup();
    const onPulled = vi.fn();
    wrap(
      <PullVersionButton
        projectId={PROJECT_ID}
        version={v(8, { server_only: true })}
        onPulled={onPulled}
      />,
    );
    await user.click(screen.getByRole('button', { name: 'Pull version 8' }));
    expect(await screen.findByText('Version 8 pulled')).toBeInTheDocument();
    expect(
      screen.getByText('It is on this machine now. Install it to use it.'),
    ).toBeInTheDocument();
    expect(onPulled).toHaveBeenCalled();
  });

  it('renders nothing for local versions', () => {
    const { container } = wrap(
      <PullVersionButton projectId={PROJECT_ID} version={v(8)} />,
    );
    expect(container.querySelector('button')).toBeNull();
  });
});

describe('SyncMarkerLabel', () => {
  it('shows the marker label', () => {
    wrap(<SyncMarkerLabel marker="newest_on_server" />);
    expect(screen.getByText('Newest on server')).toBeInTheDocument();
  });
});

describe('ConnectedServerBadge', () => {
  it('shows the server without its scheme', async () => {
    connected();
    wrap(<ConnectedServerBadge />);
    expect(await screen.findByText('nebi.example.com')).toBeInTheDocument();
  });
});

describe('trackPullToast', () => {
  it('reports a cancelled install with the version that stays', async () => {
    let polls = 0;
    server.use(
      http.get(`${C}/jobs/:jobId`, () => {
        polls += 1;
        return HttpResponse.json({
          ...mockPullJob,
          status: polls === 1 ? 'running' : 'cancelled',
          version_number: 4,
          logs: '',
        });
      }),
    );
    wrap(<div />);
    const onSettled = vi.fn();
    trackPullToast(mockPullJob, {
      projectName: 'ml-baseline',
      install: true,
      onOpenProject: () => {},
      onSettled,
      intervalMs: 20,
    });
    expect(
      await screen.findByText(
        'The install was cancelled. Version 4 is on this machine; install it when you need it.',
      ),
    ).toBeInTheDocument();
    expect(screen.getByText('ml-baseline pulled')).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Open project' }),
    ).toBeInTheDocument();
    expect(onSettled).toHaveBeenCalled();
  });

  it('shows the installing step', async () => {
    let polls = 0;
    server.use(
      http.get(`${C}/jobs/:jobId`, () => {
        polls += 1;
        return HttpResponse.json({
          ...mockPullJob,
          status: polls < 5 ? 'running' : 'completed',
          version_number: 2,
          logs: '',
        });
      }),
    );
    wrap(<div />);
    trackPullToast(mockPullJob, {
      projectName: 'demo',
      install: true,
      intervalMs: 20,
    });
    expect(
      await screen.findByText('Pulled. Installing version 2…'),
    ).toBeInTheDocument();
    expect(await screen.findByText('demo installed')).toBeInTheDocument();
  });

  it('reports a pull that failed before any version arrived', async () => {
    server.use(
      http.get(`${C}/jobs/:jobId`, () =>
        HttpResponse.json({
          ...mockPullJob,
          type: 'pull',
          status: 'failed',
          version_number: null,
          error: 'server unreachable',
          logs: '',
        }),
      ),
    );
    wrap(<div />);
    trackPullToast(mockPullJob, {
      projectName: 'demo2',
      install: false,
      intervalMs: 20,
    });
    expect(
      (await screen.findAllByText('demo2 couldn’t be pulled')).length,
    ).toBeGreaterThan(0);
  });
});
