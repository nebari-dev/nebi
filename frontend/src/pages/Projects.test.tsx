import { fireEvent, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '@/test/utils';
import { Projects } from './Projects';

const createMutateAsync = vi.fn().mockResolvedValue({ id: 'ws-1' });

vi.mock('@/hooks/useProjects', () => ({
  useProjects: () => ({ data: [], isLoading: false }),
  useCreateProject: () => ({
    mutateAsync: createMutateAsync,
    isPending: false,
  }),
  useDeleteProject: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

vi.mock('@/hooks/useRemote', () => ({
  useRemoteView: () => ({
    isLocalMode: true,
    viewMode: 'local',
    isRemoteConnected: false,
    isRemoteView: false,
  }),
  useRemoteProjects: () => ({
    data: undefined,
    isFirstLoad: false,
    isUnreachable: false,
  }),
  useCreateRemoteProject: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteRemoteProject: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

// The page tests only care about the toml value flowing in and out, not the
// editor's TOML/UI mode behavior (covered in PixiTomlEditor.test.tsx).
vi.mock('@/components/project/PixiTomlEditor', () => ({
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

const newProjectButton = () =>
  screen.queryByRole('button', { name: /new project/i });

const openCreateForm = async () => {
  const user = userEvent.setup();
  const button = newProjectButton();
  if (!button) throw new Error('New Project button not found');
  await user.click(button);
  return user;
};

describe('Projects create button', () => {
  it('hides the New Project button while the create form is open', async () => {
    renderWithProviders(<Projects />);

    expect(newProjectButton()).toBeInTheDocument();
    await openCreateForm();

    expect(screen.getByText('Create New Project')).toBeInTheDocument();
    expect(newProjectButton()).not.toBeInTheDocument();
  });

  it('shows the button again after dismissing the form with Cancel', async () => {
    renderWithProviders(<Projects />);
    const user = await openCreateForm();

    await user.click(screen.getByRole('button', { name: /cancel/i }));

    expect(screen.queryByText('Create New Project')).not.toBeInTheDocument();
    expect(newProjectButton()).toBeInTheDocument();
  });

  it('shows the button again after dismissing the form with the X close button', async () => {
    renderWithProviders(<Projects />);
    const user = await openCreateForm();

    await user.click(
      screen.getByRole('button', { name: /close create project form/i }),
    );

    expect(screen.queryByText('Create New Project')).not.toBeInTheDocument();
    expect(newProjectButton()).toBeInTheDocument();
  });

  it('shows the button again after a successful create', async () => {
    renderWithProviders(<Projects />);
    const user = await openCreateForm();

    fireEvent.change(screen.getByLabelText('pixi.toml'), {
      target: { value: '[workspace]\nname = "my-ws"\n' },
    });
    await user.click(screen.getByRole('button', { name: /create & save/i }));

    expect(createMutateAsync).toHaveBeenCalled();
    expect(screen.queryByText('Create New Project')).not.toBeInTheDocument();
    expect(newProjectButton()).toBeInTheDocument();
  });
});
