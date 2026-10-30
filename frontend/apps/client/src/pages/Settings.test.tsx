import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { openExternal } from '@/lib/openExternal';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';
import { server } from '@/test/handlers';
import { act, renderWithProviders, screen } from '@/test/utils';
import { Settings } from './Settings';

vi.mock('@/lib/openExternal', () => ({ openExternal: vi.fn() }));

const deviceAuthorization = {
  user_code: 'ABCD-EFGH',
  verification_uri: 'https://auth.example.com/device',
  verification_uri_complete:
    'https://auth.example.com/device?user_code=ABCD-EFGH',
  expires_in: 600,
  interval: 5,
};

const connectedServer = {
  url: 'https://nebi.example.com',
  username: 'alice',
  status: 'connected',
};

let serverStatus: Record<string, string>;
let connectBody: unknown;
let pollResponses: Array<() => Response>;

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  useModeStore.setState({ mode: 'local', loading: false });
  useViewModeStore.setState({ viewMode: 'local' });
  serverStatus = { url: '', username: '', status: 'disconnected' };
  connectBody = undefined;
  pollResponses = [];
  server.use(
    http.get('/api/v1/remote/server', () => HttpResponse.json(serverStatus)),
    http.post('/api/v1/remote/connect', async ({ request }) => {
      connectBody = await request.json();
      return HttpResponse.json(deviceAuthorization);
    }),
    http.post('/api/v1/remote/connect/poll', () => {
      const next = pollResponses.shift();
      return next ? next() : HttpResponse.json({ status: 'pending' });
    }),
  );
});

afterEach(() => {
  vi.useRealTimers();
  vi.mocked(openExternal).mockReset();
  useModeStore.setState({ mode: null });
});

const setup = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime.bind(vi) });

const startConnect = async (user: ReturnType<typeof setup>) => {
  renderWithProviders(<Settings />);
  await user.type(
    await screen.findByLabelText('Server URL'),
    'https://nebi.example.com',
  );
  await user.click(screen.getByRole('button', { name: 'Connect' }));
};

describe('Settings remote connection', () => {
  it('asks only for the server URL', async () => {
    renderWithProviders(<Settings />);

    expect(await screen.findByLabelText('Server URL')).toBeInTheDocument();
    expect(screen.queryByLabelText('Username')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Password')).not.toBeInTheDocument();
  });

  it('shows the device code and opens the sign-in page', async () => {
    const user = setup();
    await startConnect(user);

    expect(await screen.findByText('ABCD-EFGH')).toBeInTheDocument();
    expect(connectBody).toEqual({ url: 'https://nebi.example.com' });
    expect(screen.getByRole('status')).toHaveTextContent(
      'Waiting for approval...',
    );

    await user.click(screen.getByRole('button', { name: 'Open sign-in page' }));
    expect(openExternal).toHaveBeenCalledWith(
      deviceAuthorization.verification_uri_complete,
    );
  });

  it('falls back to the plain verification URI', async () => {
    server.use(
      http.post('/api/v1/remote/connect', () =>
        HttpResponse.json({
          ...deviceAuthorization,
          verification_uri_complete: '',
        }),
      ),
    );
    const user = setup();
    await startConnect(user);

    await user.click(
      await screen.findByRole('button', { name: 'Open sign-in page' }),
    );
    expect(openExternal).toHaveBeenCalledWith(
      deviceAuthorization.verification_uri,
    );
  });

  it('switches to the connected view once the approval lands', async () => {
    pollResponses = [
      () => HttpResponse.json({ status: 'pending', interval: 5 }),
      () => {
        serverStatus = connectedServer;
        return HttpResponse.json({
          status: 'connected',
          url: connectedServer.url,
          username: connectedServer.username,
        });
      },
    ];
    const user = setup();
    await startConnect(user);
    await screen.findByText('ABCD-EFGH');

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });

    expect(await screen.findByText('alice')).toBeInTheDocument();
    expect(screen.getByText('Connected')).toBeInTheDocument();
    expect(screen.queryByText('ABCD-EFGH')).not.toBeInTheDocument();
    expect(useViewModeStore.getState().viewMode).toBe('remote');
  });

  it('connects immediately to a server with auth disabled', async () => {
    server.use(
      http.post('/api/v1/remote/connect', () => {
        serverStatus = connectedServer;
        return HttpResponse.json({
          status: 'connected',
          url: connectedServer.url,
          username: connectedServer.username,
        });
      }),
    );
    const user = setup();
    await startConnect(user);

    expect(await screen.findByText('alice')).toBeInTheDocument();
    expect(
      screen.queryByText('Waiting for approval...'),
    ).not.toBeInTheDocument();
    expect(useViewModeStore.getState().viewMode).toBe('remote');
  });

  it('shows the error and lets the user start again when approval fails', async () => {
    pollResponses = [
      () =>
        HttpResponse.json({ error: 'device code expired' }, { status: 400 }),
    ];
    const user = setup();
    await startConnect(user);
    await screen.findByText('ABCD-EFGH');

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'device code expired',
    );
    expect(screen.getByRole('button', { name: 'Connect' })).toBeEnabled();
  });

  it('shows why the connection could not start', async () => {
    server.use(
      http.post('/api/v1/remote/connect', () =>
        HttpResponse.json(
          { error: 'remote server is unreachable' },
          { status: 502 },
        ),
      ),
    );
    const user = setup();
    await startConnect(user);

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'remote server is unreachable',
    );
  });

  it('cancels a pending approval', async () => {
    const user = setup();
    await startConnect(user);
    await screen.findByText('ABCD-EFGH');

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.queryByText('ABCD-EFGH')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Server URL')).toBeInTheDocument();
  });
});
