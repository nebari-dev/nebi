import { describe, expect, it } from 'vitest';
import { renderWithProviders, screen } from '@/test/utils';
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
});
