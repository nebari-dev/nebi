import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  mockAdminUser,
  mockCollaborator,
  mockGroupCollaborator,
  mockOwnerCollaborator,
  mockUser,
  server,
} from '@/test/handlers';
import { renderWithProviders } from '@/test/utils';
import { ShareDialog } from './ShareDialog';

const defaultProps = {
  open: true,
  onOpenChange: vi.fn(),
  environmentId: 'ws-1',
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe('ShareDialog', () => {
  it('shows a loading spinner while fetching collaborators', () => {
    // Slow the response so spinner is visible on initial render
    server.use(
      http.get('/api/v1/workspaces/:id/collaborators', async () => {
        await new Promise((r) => setTimeout(r, 500));
        return HttpResponse.json([]);
      }),
    );
    renderWithProviders(<ShareDialog {...defaultProps} />);
    expect(document.querySelector('.animate-spin')).toBeTruthy();
  });

  it('renders collaborators after loading', async () => {
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() =>
      expect(
        screen.getByText(mockOwnerCollaborator.username),
      ).toBeInTheDocument(),
    );
    expect(screen.getByText(mockCollaborator.username)).toBeInTheDocument();
  });

  it('shows role badges for each collaborator', async () => {
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() => expect(screen.getByText('Owner')).toBeInTheDocument());
    expect(screen.getByText('Viewer')).toBeInTheDocument();
  });

  it('shows no remove button for the owner', async () => {
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() =>
      expect(
        screen.getByText(mockOwnerCollaborator.username),
      ).toBeInTheDocument(),
    );
    // The owner row should have no X button — only the non-owner should
    const removeButtons = screen.getAllByRole('button', { name: '' }); // X buttons have no accessible label
    // There's 1 non-owner collaborator so 1 remove button
    expect(removeButtons).toHaveLength(1);
  });

  it('shows the Add Collaborator form when non-collaborator users exist', async () => {
    // admin-1 is not yet a collaborator (only user-1 and user-2 are)
    renderWithProviders(<ShareDialog {...defaultProps} />);
    // The submit button (not the heading) confirms the form is rendered
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Add Collaborator' }),
      ).toBeInTheDocument(),
    );
  });

  it('hides the Add Collaborator form when all users are already collaborators', async () => {
    // Make all users already collaborators
    server.use(
      http.get('/api/v1/admin/users', () =>
        HttpResponse.json([mockUser, { ...mockAdminUser, id: 'user-2' }]),
      ),
    );
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() =>
      expect(
        screen.getByText(mockOwnerCollaborator.username),
      ).toBeInTheDocument(),
    );
    // The section heading and toggle still render, but the user-form submit button does not
    expect(
      screen.queryByRole('button', { name: 'Add Collaborator' }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText('All users are already collaborators.'),
    ).toBeInTheDocument();
  });

  it('shows a confirm dialog when remove button is clicked', async () => {
    const user = userEvent.setup();
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() =>
      expect(screen.getByText(mockCollaborator.username)).toBeInTheDocument(),
    );

    // Click the X button for the non-owner collaborator
    const removeButtons = screen.getAllByRole('button');
    const removeBtn = removeButtons.find((b) => b.querySelector('svg'));
    await user.click(removeBtn!);

    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Remove Collaborator' }),
      ).toBeInTheDocument(),
    );
  });

  it('calls the unshare endpoint after confirming removal', async () => {
    const user = userEvent.setup();
    let unshareCalledWith: string | null = null;
    server.use(
      http.delete('/api/v1/workspaces/:id/share/:userId', ({ params }) => {
        unshareCalledWith = params.userId as string;
        return new HttpResponse(null, { status: 204 });
      }),
    );

    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() =>
      expect(screen.getByText(mockCollaborator.username)).toBeInTheDocument(),
    );

    const removeButtons = screen.getAllByRole('button');
    const removeBtn = removeButtons.find((b) => b.querySelector('svg'));
    await user.click(removeBtn!);

    await waitFor(() =>
      expect(screen.getByText('Remove Collaborator')).toBeInTheDocument(),
    );
    await user.click(screen.getByRole('button', { name: 'Remove' }));

    await waitFor(() =>
      expect(unshareCalledWith).toBe(mockCollaborator.user_id),
    );
  });

  it('shows an error message when unshare request fails', async () => {
    server.use(
      http.delete('/api/v1/workspaces/:id/share/:userId', () =>
        HttpResponse.json({ error: 'Permission denied' }, { status: 403 }),
      ),
    );

    const user = userEvent.setup();
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() =>
      expect(screen.getByText(mockCollaborator.username)).toBeInTheDocument(),
    );

    // Click the remove button and confirm
    const removeButtons = screen.getAllByRole('button');
    const removeBtn = removeButtons.find((b) => b.querySelector('svg'));
    await user.click(removeBtn!);
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Remove Collaborator' }),
      ).toBeInTheDocument(),
    );
    await user.click(screen.getByRole('button', { name: 'Remove' }));

    await waitFor(() =>
      expect(screen.getByText('Permission denied')).toBeInTheDocument(),
    );
  });

  it('switches to group mode and shows the group form', async () => {
    // useIsAdmin returns true in tests (default /admin/users handler returns 200),
    // so ShareDialog sources the picker from /admin/groups. Mock both endpoints
    // so the test works regardless of admin-detection drift.
    const group = {
      id: 'g-1',
      name: 'data-science',
      description: '',
      source: 'native',
      created_at: '',
      updated_at: '',
    };
    server.use(
      http.get('/api/v1/groups/me', () => HttpResponse.json([group])),
      http.get('/api/v1/admin/groups', () =>
        HttpResponse.json([{ ...group, member_count: 0 }]),
      ),
    );
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() =>
      expect(
        screen.getByText(mockOwnerCollaborator.username),
      ).toBeInTheDocument(),
    );

    await userEvent.click(screen.getByRole('button', { name: 'Group' }));

    // After switching to group mode the group placeholder and submit button render
    await waitFor(() =>
      expect(screen.getByText(/Select group/i)).toBeInTheDocument(),
    );
    expect(
      screen.getByRole('button', { name: /Add Group/ }),
    ).toBeInTheDocument();
  });

  it('renders a group collaborator with its source badge', async () => {
    server.use(
      http.get('/api/v1/workspaces/:id/collaborators', () =>
        HttpResponse.json([mockOwnerCollaborator, mockGroupCollaborator]),
      ),
    );
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() =>
      expect(screen.getByText('data-science')).toBeInTheDocument(),
    );
    expect(screen.getByText(/Native group/i)).toBeInTheDocument();
  });
});
