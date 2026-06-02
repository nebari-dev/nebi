import { describe, expect, it, vi } from 'vitest';
import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/utils';
import { UseLocallyButton } from './UseLocallyButton';

describe('UseLocallyButton', () => {
  it('opens the local usage panel and copies the pull command', async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });
    const command = `nebi login ${window.location.origin} && nebi pull ml-workspace`;

    renderWithProviders(<UseLocallyButton workspaceName="ml-workspace" />);

    await user.click(screen.getByRole('button', { name: /use locally/i }));

    expect(screen.getByRole('dialog', { name: /use this workspace locally/i })).toBeInTheDocument();
    expect(screen.getByText(command)).toBeInTheDocument();
    expect(screen.getByText('No nebi CLI?')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /install it/i })).toHaveClass('text-primary');

    await user.click(screen.getByRole('button', { name: /copy nebi pull command/i }));

    expect(writeText).toHaveBeenCalledWith(command);
    expect(screen.getByRole('button', { name: /copied nebi pull command/i })).toBeInTheDocument();
  });
});
