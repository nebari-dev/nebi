import { Toaster } from '@nebi/ui/components/ui/toast';
import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import {
  mockServerConnection,
  PROJECT_ID,
  SERVER_PROJECT_ID,
} from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import { mockPullJob, remoteConnectedHandlers } from '@/test/handlers/remote';
import type { ConnectRequest } from '@/types/contracts';
import { ServerPage } from './ServerPage';

const C = '/api/v1/client';

function renderServer(path = '/server') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <Toaster>
          <MemoryRouter initialEntries={[path]}>
            <Routes>
              <Route path="/server" element={<ServerPage />} />
              <Route path="/projects/:id" element={<h1>Project page</h1>} />
            </Routes>
          </MemoryRouter>
        </Toaster>
      </TooltipProvider>
    </QueryClientProvider>,
  );
}

describe('ServerPage', () => {
  it('shows the disconnected state and connects with a token', async () => {
    const user = userEvent.setup();
    let sent: ConnectRequest | undefined;
    server.use(
      http.get(`${C}/remote/server`, () =>
        HttpResponse.json(
          sent
            ? mockServerConnection
            : { ...mockServerConnection, connected: false, url: '' },
        ),
      ),
      http.put(`${C}/remote/server`, async ({ request }) => {
        sent = (await request.json()) as ConnectRequest;
        return HttpResponse.json(mockServerConnection);
      }),
      http.get(`${C}/remote/projects`, () =>
        HttpResponse.json({ projects: [], last_synced_at: null }),
      ),
    );
    renderServer();

    expect(
      await screen.findByRole('heading', { name: 'No server connected' }),
    ).toBeInTheDocument();
    expect(screen.getByText('Not connected')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Connect server' }));

    const dialog = await screen.findByRole('dialog');
    expect(
      within(dialog).getByText('You can connect to one server at a time.'),
    ).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Connect' }));
    expect(
      await within(dialog).findByText('Enter the server’s URL.'),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByText('Paste a token, or choose None.'),
    ).toBeInTheDocument();

    await user.type(within(dialog).getByLabelText('Server URL'), 'not a url');
    await user.click(within(dialog).getByRole('button', { name: 'Connect' }));
    expect(
      await within(dialog).findByText(
        'That doesn’t look like a URL. Try nebi.example.com.',
      ),
    ).toBeInTheDocument();

    await user.clear(within(dialog).getByLabelText('Server URL'));
    await user.type(
      within(dialog).getByLabelText('Server URL'),
      'nebi.example.com',
    );
    await user.type(within(dialog).getByLabelText('Token'), 'secret');
    await user.click(within(dialog).getByRole('button', { name: 'Connect' }));

    expect(
      await screen.findByText('Connected to nebi.example.com'),
    ).toBeInTheDocument();
    expect(sent).toEqual({
      url: 'nebi.example.com',
      auth: 'token',
      token: 'secret',
    });
    expect(await screen.findByText('Signed in as')).toBeInTheDocument();
    expect(
      await screen.findByText('Nothing is shared with you on this server yet.'),
    ).toBeInTheDocument();
  });

  it('shows the field error the backend returns', async () => {
    const user = userEvent.setup();
    server.use(
      http.put(`${C}/remote/server`, () =>
        HttpResponse.json(
          {
            error: 'The server didn’t accept this token.',
            code: 'invalid',
            field: 'token',
          },
          { status: 422 },
        ),
      ),
    );
    renderServer('/server?dialog=connect');
    const dialog = await screen.findByRole('dialog');
    await user.type(
      within(dialog).getByLabelText('Server URL'),
      'nebi.example.com',
    );
    await user.type(within(dialog).getByLabelText('Token'), 'bad');
    await user.click(within(dialog).getByRole('button', { name: 'Connect' }));
    expect(
      await within(dialog).findByText('The server didn’t accept this token.'),
    ).toBeInTheDocument();
  });

  it('lists server projects with search, sort and refresh', async () => {
    const user = userEvent.setup();
    server.use(...remoteConnectedHandlers);
    renderServer();

    expect(await screen.findByText('Connected to')).toBeInTheDocument();
    expect(screen.getAllByText('nebi.example.com').length).toBeGreaterThan(0);
    expect(screen.getByText('sam')).toBeInTheDocument();
    expect(await screen.findByText('ml-baseline')).toBeInTheDocument();
    expect(await screen.findByText('2 minutes ago')).toBeInTheDocument();

    const mlRow = screen.getByText('ml-baseline').closest('tr') as HTMLElement;
    expect(
      within(mlRow).getByRole('button', { name: 'Pull and install' }),
    ).toBeInTheDocument();
    const p1Row = screen.getByText('Project_1').closest('tr') as HTMLElement;
    expect(
      within(p1Row).getByRole('button', { name: 'Open project' }),
    ).toBeInTheDocument();

    await user.type(
      screen.getByRole('searchbox', { name: 'Search server projects' }),
      'zzz',
    );
    expect(
      screen.getByText('No server projects match “zzz”.'),
    ).toBeInTheDocument();
    await user.clear(
      screen.getByRole('searchbox', { name: 'Search server projects' }),
    );

    // Sort by latest version, descending.
    await user.click(screen.getByRole('button', { name: /Latest version/ }));
    await user.click(screen.getByRole('button', { name: /Latest version/ }));
    const rows = screen.getAllByRole('row').slice(1);
    expect(rows[0]).toHaveTextContent('Project_1');

    await user.click(
      screen.getByRole('button', { name: 'Refresh the server’s project list' }),
    );
    expect(await screen.findByText('Just now')).toBeInTheDocument();
  });

  it('opens a project that is already on this machine', async () => {
    const user = userEvent.setup();
    server.use(...remoteConnectedHandlers);
    renderServer();
    await user.click(
      await screen.findByRole('button', { name: 'Open project' }),
    );
    expect(
      await screen.findByRole('heading', { name: 'Project page' }),
    ).toBeInTheDocument();
  });

  it('manages and disconnects the connection', async () => {
    const user = userEvent.setup();
    let disconnected = false;
    server.use(
      http.get(`${C}/remote/server`, () =>
        HttpResponse.json(
          disconnected
            ? { ...mockServerConnection, connected: false, url: '' }
            : mockServerConnection,
        ),
      ),
      ...remoteConnectedHandlers,
      http.delete(`${C}/remote/server`, () => {
        disconnected = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderServer();
    await user.click(
      await screen.findByRole('button', { name: 'Manage connection' }),
    );
    const dialog = await screen.findByRole('dialog');
    expect(
      within(dialog).getByRole('heading', { name: 'Manage connection' }),
    ).toBeInTheDocument();
    expect(within(dialog).getByLabelText('Server URL')).toHaveValue(
      'nebi.example.com',
    );
    expect(
      within(dialog).getByRole('button', { name: 'Save changes' }),
    ).toBeInTheDocument();
    await user.click(
      within(dialog).getByRole('button', { name: 'Disconnect' }),
    );

    const confirm = await screen.findByRole('alertdialog');
    expect(
      within(confirm).getByRole('heading', {
        name: 'Disconnect from nebi.example.com?',
      }),
    ).toBeInTheDocument();
    await user.click(
      within(confirm).getByRole('button', { name: 'Disconnect' }),
    );
    expect(
      await screen.findByText('Disconnected from nebi.example.com'),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        'Projects you pulled stay on this machine and keep working.',
      ),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole('heading', { name: 'No server connected' }),
    ).toBeInTheDocument();
  });

  it('saves the connection without retyping the token', async () => {
    const user = userEvent.setup();
    let sent: ConnectRequest | undefined;
    server.use(
      ...remoteConnectedHandlers,
      http.put(`${C}/remote/server`, async ({ request }) => {
        sent = (await request.json()) as ConnectRequest;
        return HttpResponse.json(mockServerConnection);
      }),
    );
    renderServer('/server?dialog=manage');
    const dialog = await screen.findByRole('dialog');
    await user.click(
      within(dialog).getByRole('button', { name: 'Save changes' }),
    );
    expect(await screen.findByText('Connection updated')).toBeInTheDocument();
    expect(sent).toEqual({
      url: mockServerConnection.url,
      auth: 'token',
      token: '',
    });
  });

  it('pulls and installs with a local name and follows the job', async () => {
    const user = userEvent.setup();
    let polls = 0;
    server.use(
      ...remoteConnectedHandlers,
      http.post(`${C}/remote/projects/:serverProjectId/pull`, () =>
        HttpResponse.json(
          { project_id: PROJECT_ID, job: mockPullJob },
          { status: 202 },
        ),
      ),
      http.get(`${C}/jobs/:jobId`, () => {
        polls += 1;
        if (polls === 1)
          return HttpResponse.json({
            ...mockPullJob,
            status: 'running',
            version_number: 3,
            logs: '',
          });
        return HttpResponse.json({
          ...mockPullJob,
          status: 'completed',
          version_number: 3,
          logs: '',
        });
      }),
    );
    renderServer();
    const row = (await screen.findByText('ml-baseline')).closest(
      'tr',
    ) as HTMLElement;
    await user.click(
      within(row).getByRole('button', { name: 'Pull and install' }),
    );
    const dialog = await screen.findByRole('dialog');
    expect(
      within(dialog).getByRole('heading', {
        name: 'Pull and install ml-baseline',
      }),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByText(
        /Choose a name for this project on your machine\. The environment will install after pulling\./,
      ),
    ).toBeInTheDocument();
    const name = within(dialog).getByLabelText('Local name');
    expect(name).toHaveValue('ml-baseline');

    // Collides with the local Project_1 (case-insensitive).
    await user.clear(name);
    await user.type(name, 'project_1');
    await user.click(
      within(dialog).getByRole('button', { name: 'Pull and install' }),
    );
    expect(
      await within(dialog).findByText(
        'A project called project_1 is already on this machine. Pick another name.',
      ),
    ).toBeInTheDocument();

    await user.clear(name);
    await user.click(
      within(dialog).getByRole('button', { name: 'Pull and install' }),
    );
    expect(
      await within(dialog).findByText('Give the project a name.'),
    ).toBeInTheDocument();

    await user.type(name, 'ml-local');
    await user.click(
      within(dialog).getByRole('button', { name: 'Pull and install' }),
    );
    expect(
      await screen.findByText('Pull and install job started'),
    ).toBeInTheDocument();
    expect(
      await screen.findByText(
        'Version 3 is ready to use on this machine.',
        {},
        { timeout: 4000 },
      ),
    ).toBeInTheDocument();
    expect(screen.getByText('ml-baseline installed')).toBeInTheDocument();
  });

  it('shows the backend name check', async () => {
    const user = userEvent.setup();
    server.use(
      ...remoteConnectedHandlers,
      http.post(`${C}/remote/projects/:serverProjectId/pull`, () =>
        HttpResponse.json(
          {
            error:
              'A project called taken is already on this machine. Pick another name.',
            code: 'name_taken',
          },
          { status: 409 },
        ),
      ),
    );
    renderServer();
    const row = (await screen.findByText('ml-baseline')).closest(
      'tr',
    ) as HTMLElement;
    await user.click(
      within(row).getByRole('button', {
        name: 'More actions for ml-baseline',
      }),
    );
    await user.click(
      await screen.findByRole('menuitem', { name: 'Pull project' }),
    );
    const dialog = await screen.findByRole('dialog');
    expect(
      within(dialog).getByRole('heading', { name: 'Pull ml-baseline' }),
    ).toBeInTheDocument();
    const name = within(dialog).getByLabelText('Local name');
    await user.clear(name);
    await user.type(name, 'taken');
    await user.click(within(dialog).getByRole('button', { name: 'Pull' }));
    expect(
      await within(dialog).findByText(
        'A project called taken is already on this machine. Pick another name.',
      ),
    ).toBeInTheDocument();
  });

  it('shows an unreachable server and keeps local work', async () => {
    server.use(
      http.get(`${C}/remote/server`, () =>
        HttpResponse.json(mockServerConnection),
      ),
      http.get(`${C}/remote/projects`, () =>
        HttpResponse.json(
          { error: 'server unreachable', code: 'server_unreachable' },
          { status: 502 },
        ),
      ),
    );
    renderServer();
    expect(
      await screen.findByRole('heading', {
        name: 'Can’t reach nebi.example.com',
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Try again' }),
    ).toBeInTheDocument();
  });

  it('opens the pull dialog from a deep link', async () => {
    server.use(...remoteConnectedHandlers);
    renderServer(`/server?dialog=pull-install&project=${SERVER_PROJECT_ID}`);
    const dialog = await screen.findByRole('dialog');
    await waitFor(() =>
      expect(
        within(dialog).getByRole('heading', {
          name: 'Pull and install ml-baseline',
        }),
      ).toBeInTheDocument(),
    );
  });
});
