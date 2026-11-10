import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { Toaster, toast } from './toast';

describe('Toaster', () => {
  it('shows a toast with an Undo action', async () => {
    const undo = vi.fn();
    render(<Toaster />);
    act(() => {
      toast.add({
        type: 'success',
        title: 'Access removed',
        actionProps: { children: 'Undo', onClick: undo },
      });
    });
    expect(await screen.findByText('Access removed')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Undo' }));
    expect(undo).toHaveBeenCalledOnce();
    expect(screen.getByRole('button', { name: 'Dismiss' })).toBeInTheDocument();
  });

  it('stacks above dialogs so an Undo stays clickable while one is open', async () => {
    render(<Toaster />);
    act(() => {
      toast.add({ type: 'success', title: 'Access updated' });
    });
    await screen.findByText('Access updated');
    const viewport = document.querySelector('[data-slot="toast-viewport"]');
    // Dialog viewports and backdrops use z-50.
    expect(viewport?.className).toContain('z-[60]');
  });
});
