import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { mockUser } from '@/test/handlers';
import { render, screen } from '@/test/utils';
import { ProfileMenu } from './ProfileMenu';

describe('ProfileMenu', () => {
  it('opens a profile menu with the current user and sign out action', async () => {
    const user = userEvent.setup();

    render(<ProfileMenu user={mockUser} onLogout={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: /testuser/i }));

    expect(screen.getByRole('menu')).toBeInTheDocument();
    expect(screen.getByText('test@example.com')).toBeInTheDocument();
    expect(
      screen.getByRole('menuitem', { name: /sign out/i }),
    ).toBeInTheDocument();
  });

  it('closes on outside click and Escape', async () => {
    const user = userEvent.setup();

    render(
      <>
        <button type="button">Outside</button>
        <ProfileMenu user={mockUser} onLogout={vi.fn()} />
      </>,
    );

    await user.click(screen.getByRole('button', { name: /testuser/i }));
    expect(screen.getByRole('menu')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Outside' }));
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();

    const trigger = screen.getByRole('button', { name: /testuser/i });
    await user.click(trigger);
    expect(screen.getByRole('menu')).toBeInTheDocument();

    await user.keyboard('{Escape}');

    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it('supports tabbing to sign out and activating it with the keyboard', async () => {
    const user = userEvent.setup();
    const onLogout = vi.fn();

    render(<ProfileMenu user={mockUser} onLogout={onLogout} />);

    screen.getByRole('button', { name: /testuser/i }).focus();
    await user.keyboard('{Enter}');

    const signOut = screen.getByRole('menuitem', { name: /sign out/i });
    await user.tab();
    expect(signOut).toHaveFocus();

    await user.keyboard('{Enter}');

    expect(onLogout).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });
});
