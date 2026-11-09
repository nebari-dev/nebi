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
});
