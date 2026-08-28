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

  it('shows a labeled Install button when not installed', () => {
    renderWithProviders(
      <InstallControls workspaceId="ws-1" installStatus="not_installed" />,
    );
    const button = screen.getByRole('button', { name: /install/i });
    expect(button).toHaveTextContent('Install');
  });

  it('shows a labeled Retry Install button after a failed install', () => {
    renderWithProviders(
      <InstallControls workspaceId="ws-1" installStatus="install_failed" />,
    );
    const button = screen.getByRole('button', { name: /install/i });
    expect(button).toHaveTextContent('Retry Install');
  });

  it('shows a labeled Uninstall button when installed', () => {
    renderWithProviders(
      <InstallControls workspaceId="ws-1" installStatus="installed" />,
    );
    const button = screen.getByRole('button', { name: /uninstall/i });
    expect(button).toHaveTextContent('Uninstall');
  });

  it('shows a disabled progress indicator while installing', () => {
    renderWithProviders(
      <InstallControls workspaceId="ws-1" installStatus="installing" />,
    );
    const button = screen.getByRole('button', { name: /installing/i });
    expect(button).toBeDisabled();
    expect(button).toHaveTextContent('Installing...');
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

  describe('icon appearance (table rows)', () => {
    it('renders an icon-only install button with an aria-label and title', () => {
      renderWithProviders(
        <InstallControls
          workspaceId="ws-1"
          installStatus="not_installed"
          appearance="icon"
        />,
      );
      const button = screen.getByRole('button', {
        name: 'Install environment',
      });
      expect(button).toHaveTextContent('');
      expect(button).toHaveAttribute(
        'title',
        'Download and install packages from the lockfile',
      );
    });

    it('renders an icon-only uninstall button with an aria-label and title', () => {
      renderWithProviders(
        <InstallControls
          workspaceId="ws-1"
          installStatus="installed"
          appearance="icon"
        />,
      );
      const button = screen.getByRole('button', {
        name: 'Uninstall environment',
      });
      expect(button).toHaveTextContent('');
      expect(button).toHaveAttribute('title');
    });

    it('keeps the pending state distinguishable while installing', () => {
      renderWithProviders(
        <InstallControls
          workspaceId="ws-1"
          installStatus="installing"
          appearance="icon"
        />,
      );
      const button = screen.getByRole('button', { name: 'Installing...' });
      expect(button).toBeDisabled();
      expect(button).toHaveAttribute('title', 'Installing...');
    });

    it('keeps the pending state distinguishable while uninstalling', () => {
      renderWithProviders(
        <InstallControls
          workspaceId="ws-1"
          installStatus="uninstalling"
          appearance="icon"
        />,
      );
      const button = screen.getByRole('button', { name: 'Uninstalling...' });
      expect(button).toBeDisabled();
    });

    it('still asks for confirmation before uninstalling', async () => {
      renderWithProviders(
        <InstallControls
          workspaceId="ws-1"
          installStatus="installed"
          appearance="icon"
        />,
      );
      screen.getByRole('button', { name: 'Uninstall environment' }).click();
      expect(
        await screen.findByRole('alertdialog', { name: /uninstall/i }),
      ).toBeInTheDocument();
    });

    it('stops click propagation so the row does not navigate', async () => {
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
      const onRowClick = vi.fn();
      renderWithProviders(
        // biome-ignore lint/a11y/useKeyWithClickEvents: test-only click capture stand-in for the table row
        // biome-ignore lint/a11y/noStaticElementInteractions: test-only click capture stand-in for the table row
        <div onClick={onRowClick}>
          <InstallControls
            workspaceId="ws-1"
            installStatus="not_installed"
            appearance="icon"
          />
        </div>,
      );
      screen.getByRole('button', { name: 'Install environment' }).click();
      expect(onRowClick).not.toHaveBeenCalled();
    });
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
