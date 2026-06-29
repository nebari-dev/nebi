import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { mockUser } from '@/test/handlers';
import { render, screen } from '@/test/utils';
import { ProfileMenu } from './ProfileMenu';

const renderProfileMenu = ({
  onLogout = vi.fn(),
  onThemeChange = vi.fn(),
  themeMode = 'system' as const,
} = {}) =>
  render(
    <ProfileMenu
      user={mockUser}
      themeMode={themeMode}
      onThemeChange={onThemeChange}
      onLogout={onLogout}
    />,
  );

describe('ProfileMenu', () => {
  it('opens a profile menu with the current user, theme control, and sign out action', async () => {
    const user = userEvent.setup();

    renderProfileMenu();

    await user.click(screen.getByRole('button', { name: /testuser/i }));

    expect(screen.getByRole('menu')).toBeInTheDocument();
    expect(screen.getByText('test@example.com')).toBeInTheDocument();
    expect(screen.getByRole('group', { name: /theme/i })).toBeInTheDocument();
    expect(
      screen.getByRole('menuitemradio', { name: /system theme/i }),
    ).toHaveAttribute('aria-checked', 'true');
    expect(
      screen.getByRole('menuitem', { name: /sign out/i }),
    ).toBeInTheDocument();
  });

  it('closes on outside click and Escape', async () => {
    const user = userEvent.setup();

    render(
      <>
        <button type="button">Outside</button>
        <ProfileMenu
          user={mockUser}
          themeMode="system"
          onThemeChange={vi.fn()}
          onLogout={vi.fn()}
        />
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

  it('selects a theme without closing the menu', async () => {
    const user = userEvent.setup();
    const onThemeChange = vi.fn();

    const { rerender } = renderProfileMenu({ onThemeChange });

    await user.click(screen.getByRole('button', { name: /testuser/i }));
    await user.click(screen.getByRole('menuitemradio', { name: /dark mode/i }));

    expect(onThemeChange).toHaveBeenCalledWith('dark');
    expect(screen.getByRole('menu')).toBeInTheDocument();

    rerender(
      <ProfileMenu
        user={mockUser}
        themeMode="dark"
        onThemeChange={onThemeChange}
        onLogout={vi.fn()}
      />,
    );

    expect(
      screen.getByRole('menuitemradio', { name: /dark mode/i }),
    ).toHaveAttribute('aria-checked', 'true');
  });

  it('supports tabbing to sign out and activating it with the keyboard', async () => {
    const user = userEvent.setup();
    const onLogout = vi.fn();

    renderProfileMenu({ onLogout });

    screen.getByRole('button', { name: /testuser/i }).focus();
    await user.keyboard('{Enter}');

    await user.tab();
    expect(
      screen.getByRole('menuitemradio', { name: /light mode/i }),
    ).toHaveFocus();

    await user.tab();
    expect(
      screen.getByRole('menuitemradio', { name: /dark mode/i }),
    ).toHaveFocus();

    await user.tab();
    expect(
      screen.getByRole('menuitemradio', { name: /system theme/i }),
    ).toHaveFocus();

    const signOut = screen.getByRole('menuitem', { name: /sign out/i });
    await user.tab();
    expect(signOut).toHaveFocus();

    await user.keyboard('{Enter}');

    expect(onLogout).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });
});
