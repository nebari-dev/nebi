import { HttpResponse, http } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { server } from '@/test/handlers';
import { renderWithProviders, screen, waitFor } from '@/test/utils';
import { InstallControls } from './InstallControls';

describe('InstallControls', () => {
  it('renders nothing when install status is absent (team mode)', () => {
    const { container } = renderWithProviders(
      <InstallControls workspaceId="ws-1" installStatus={undefined} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('shows an Install button when not installed', () => {
    renderWithProviders(
      <InstallControls workspaceId="ws-1" installStatus="not_installed" />,
    );
    expect(
      screen.getByRole('button', { name: /install/i }),
    ).toBeInTheDocument();
  });

  it('shows an Install button after a failed install', () => {
    renderWithProviders(
      <InstallControls workspaceId="ws-1" installStatus="install_failed" />,
    );
    expect(
      screen.getByRole('button', { name: /install/i }),
    ).toBeInTheDocument();
  });

  it('shows an Uninstall button when installed', () => {
    renderWithProviders(
      <InstallControls workspaceId="ws-1" installStatus="installed" />,
    );
    expect(
      screen.getByRole('button', { name: /uninstall/i }),
    ).toBeInTheDocument();
  });

  it('shows a disabled progress indicator while installing', () => {
    renderWithProviders(
      <InstallControls workspaceId="ws-1" installStatus="installing" />,
    );
    const button = screen.getByRole('button', { name: /installing/i });
    expect(button).toBeDisabled();
  });

  it('calls onStarted with the queued job after clicking Install', async () => {
    server.use(
      http.post('/api/v1/workspaces/ws-1/install', () =>
        HttpResponse.json(
          {
            id: 'job-1',
            workspace_id: 'ws-1',
            type: 'env_install',
            status: 'pending',
          },
          { status: 202 },
        ),
      ),
    );
    const onStarted = vi.fn();
    renderWithProviders(
      <InstallControls
        workspaceId="ws-1"
        installStatus="not_installed"
        onStarted={onStarted}
      />,
    );
    screen.getByRole('button', { name: /install/i }).click();
    await waitFor(() => expect(onStarted).toHaveBeenCalled());
    expect(onStarted.mock.calls[0][0]).toMatchObject({ id: 'job-1' });
  });

  it('asks for confirmation before uninstalling', async () => {
    const uninstallCalled = vi.fn();
    server.use(
      http.post('/api/v1/workspaces/ws-1/uninstall', () => {
        uninstallCalled();
        return HttpResponse.json(
          {
            id: 'job-2',
            workspace_id: 'ws-1',
            type: 'env_uninstall',
            status: 'pending',
          },
          { status: 202 },
        );
      }),
    );
    renderWithProviders(
      <InstallControls workspaceId="ws-1" installStatus="installed" />,
    );
    screen.getByRole('button', { name: /uninstall/i }).click();
    expect(
      await screen.findByRole('alertdialog', { name: /uninstall/i }),
    ).toBeInTheDocument();
    expect(uninstallCalled).not.toHaveBeenCalled();
  });

  it('starts the uninstall and calls onStarted after confirming', async () => {
    server.use(
      http.post('/api/v1/workspaces/ws-1/uninstall', () =>
        HttpResponse.json(
          {
            id: 'job-2',
            workspace_id: 'ws-1',
            type: 'env_uninstall',
            status: 'pending',
          },
          { status: 202 },
        ),
      ),
    );
    const onStarted = vi.fn();
    renderWithProviders(
      <InstallControls
        workspaceId="ws-1"
        installStatus="installed"
        onStarted={onStarted}
      />,
    );
    screen.getByRole('button', { name: /uninstall environment/i }).click();
    (await screen.findByRole('button', { name: 'Uninstall' })).click();
    await waitFor(() => expect(onStarted).toHaveBeenCalled());
    expect(onStarted.mock.calls[0][0]).toMatchObject({ id: 'job-2' });
  });

  it('does not uninstall when the confirmation is cancelled', async () => {
    const uninstallCalled = vi.fn();
    server.use(
      http.post('/api/v1/workspaces/ws-1/uninstall', () => {
        uninstallCalled();
        return HttpResponse.json({}, { status: 202 });
      }),
    );
    renderWithProviders(
      <InstallControls workspaceId="ws-1" installStatus="installed" />,
    );
    screen.getByRole('button', { name: /uninstall environment/i }).click();
    (await screen.findByRole('button', { name: 'Cancel' })).click();
    await waitFor(() =>
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument(),
    );
    expect(uninstallCalled).not.toHaveBeenCalled();
  });
});
