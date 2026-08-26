import { fireEvent, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '@/test/utils';
import { Workspaces } from './Workspaces';

const createMutateAsync = vi.fn().mockResolvedValue({ id: 'ws-1' });

vi.mock('@/hooks/useWorkspaces', () => ({
  useWorkspaces: () => ({ data: [], isLoading: false }),
  useCreateWorkspace: () => ({
    mutateAsync: createMutateAsync,
    isPending: false,
  }),
  useDeleteWorkspace: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

vi.mock('@/hooks/useRemote', () => ({
  useRemoteView: () => ({
    isLocalMode: true,
    viewMode: 'local',
    isRemoteConnected: false,
    isRemoteView: false,
  }),
  useRemoteWorkspaces: () => ({
    data: undefined,
    isFirstLoad: false,
    isUnreachable: false,
  }),
  useCreateRemoteWorkspace: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteRemoteWorkspace: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

// The page tests only care about the toml value flowing in and out, not the
// editor's TOML/UI mode behavior (covered in PixiTomlEditor.test.tsx).
vi.mock('@/components/workspace/PixiTomlEditor', () => ({
  PixiTomlEditor: ({
    tomlValue,
    onTomlChange,
  }: {
    tomlValue: string;
    onTomlChange: (toml: string) => void;
  }) => (
    <textarea
      aria-label="pixi.toml"
      value={tomlValue}
      onChange={(e) => onTomlChange(e.target.value)}
    />
  ),
}));

const newWorkspaceButton = () =>
  screen.queryByRole('button', { name: /new workspace/i });

const openCreateForm = async () => {
  const user = userEvent.setup();
  const button = newWorkspaceButton();
  if (!button) throw new Error('New Workspace button not found');
  await user.click(button);
  return user;
};

describe('Workspaces create button', () => {
  it('hides the New Workspace button while the create form is open', async () => {
    renderWithProviders(<Workspaces />);

    expect(newWorkspaceButton()).toBeInTheDocument();
    await openCreateForm();

    expect(screen.getByText('Create New Workspace')).toBeInTheDocument();
    expect(newWorkspaceButton()).not.toBeInTheDocument();
  });

  it('shows the button again after dismissing the form with Cancel', async () => {
    renderWithProviders(<Workspaces />);
    const user = await openCreateForm();

    await user.click(screen.getByRole('button', { name: /cancel/i }));

    expect(screen.queryByText('Create New Workspace')).not.toBeInTheDocument();
    expect(newWorkspaceButton()).toBeInTheDocument();
  });

  it('shows the button again after dismissing the form with the X close button', async () => {
    renderWithProviders(<Workspaces />);
    const user = await openCreateForm();

    await user.click(
      screen.getByRole('button', { name: /close create workspace form/i }),
    );

    expect(screen.queryByText('Create New Workspace')).not.toBeInTheDocument();
    expect(newWorkspaceButton()).toBeInTheDocument();
  });

  it('shows the button again after a successful create', async () => {
    renderWithProviders(<Workspaces />);
    const user = await openCreateForm();

    fireEvent.change(screen.getByLabelText('pixi.toml'), {
      target: { value: '[workspace]\nname = "my-ws"\n' },
    });
    await user.click(screen.getByRole('button', { name: /create & save/i }));

    expect(createMutateAsync).toHaveBeenCalled();
    expect(screen.queryByText('Create New Workspace')).not.toBeInTheDocument();
    expect(newWorkspaceButton()).toBeInTheDocument();
  });
});
