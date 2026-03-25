import { describe, it, expect, vi } from 'vitest';
import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ConfirmDialog } from './confirm-dialog';
import { renderWithProviders } from '@/test/utils';

function renderDialog(overrides?: Partial<Parameters<typeof ConfirmDialog>[0]>) {
  const props = {
    open: true,
    onOpenChange: vi.fn(),
    onConfirm: vi.fn(),
    title: 'Delete item',
    description: 'This cannot be undone.',
    ...overrides,
  };
  renderWithProviders(<ConfirmDialog {...props} />);
  return props;
}

describe('ConfirmDialog', () => {
  it('renders the title and description', () => {
    renderDialog();
    expect(screen.getByText('Delete item')).toBeInTheDocument();
    expect(screen.getByText('This cannot be undone.')).toBeInTheDocument();
  });

  it('shows default button labels', () => {
    renderDialog();
    expect(screen.getByRole('button', { name: 'Continue' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
  });

  it('respects custom button labels', () => {
    renderDialog({ confirmText: 'Delete', cancelText: 'Go back' });
    expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Go back' })).toBeInTheDocument();
  });

  it('calls onConfirm and closes when confirm button is clicked', async () => {
    const user = userEvent.setup();
    const props = renderDialog();
    await user.click(screen.getByRole('button', { name: 'Continue' }));
    expect(props.onConfirm).toHaveBeenCalledOnce();
    expect(props.onOpenChange).toHaveBeenCalledWith(false);
  });

  it('does not call onConfirm when cancel is clicked', async () => {
    const user = userEvent.setup();
    const props = renderDialog();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(props.onConfirm).not.toHaveBeenCalled();
  });

  it('does not render when open is false', () => {
    renderDialog({ open: false });
    expect(screen.queryByText('Delete item')).not.toBeInTheDocument();
  });

  it('applies destructive styling class for destructive variant', () => {
    renderDialog({ variant: 'destructive', confirmText: 'Remove' });
    const btn = screen.getByRole('button', { name: 'Remove' });
    expect(btn.className).toContain('bg-red-600');
  });
});
