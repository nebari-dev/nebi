import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server, mockOwnerCollaborator, mockCollaborator, mockUser, mockAdminUser } from '@/test/handlers';
import { ShareDialog } from './ShareDialog';
import { renderWithProviders } from '@/test/utils';

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
      })
    );
    renderWithProviders(<ShareDialog {...defaultProps} />);
    expect(document.querySelector('.animate-spin')).toBeTruthy();
  });

  it('renders collaborators after loading', async () => {
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() => expect(screen.getByText(mockOwnerCollaborator.username)).toBeInTheDocument());
    expect(screen.getByText(mockCollaborator.username)).toBeInTheDocument();
  });

  it('shows role badges for each collaborator', async () => {
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() => expect(screen.getByText('Owner')).toBeInTheDocument());
    expect(screen.getByText('Viewer')).toBeInTheDocument();
  });

  it('shows no remove button for the owner', async () => {
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() => expect(screen.getByText(mockOwnerCollaborator.username)).toBeInTheDocument());
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
      expect(screen.getByRole('button', { name: 'Add Collaborator' })).toBeInTheDocument()
    );
  });

  it('hides the Add Collaborator form when all users are already collaborators', async () => {
    // Make all users already collaborators
    server.use(
      http.get('/api/v1/admin/users', () =>
        HttpResponse.json([mockUser, { ...mockAdminUser, id: 'user-2' }])
      )
    );
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() => expect(screen.getByText(mockOwnerCollaborator.username)).toBeInTheDocument());
    expect(screen.queryByText('Add Collaborator')).not.toBeInTheDocument();
  });

  it('shows a confirm dialog when remove button is clicked', async () => {
    const user = userEvent.setup();
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() => expect(screen.getByText(mockCollaborator.username)).toBeInTheDocument());

    // Click the X button for the non-owner collaborator
    const removeButtons = screen.getAllByRole('button');
    const removeBtn = removeButtons.find((b) => b.querySelector('svg'));
    await user.click(removeBtn!);

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Remove Collaborator' })).toBeInTheDocument()
    );
  });

  it('calls the unshare endpoint after confirming removal', async () => {
    const user = userEvent.setup();
    let unshareCalledWith: string | null = null;
    server.use(
      http.delete('/api/v1/workspaces/:id/share/:userId', ({ params }) => {
        unshareCalledWith = params.userId as string;
        return new HttpResponse(null, { status: 204 });
      })
    );

    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() => expect(screen.getByText(mockCollaborator.username)).toBeInTheDocument());

    const removeButtons = screen.getAllByRole('button');
    const removeBtn = removeButtons.find((b) => b.querySelector('svg'));
    await user.click(removeBtn!);

    await waitFor(() => expect(screen.getByText('Remove Collaborator')).toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: 'Remove' }));

    await waitFor(() => expect(unshareCalledWith).toBe(mockCollaborator.user_id));
  });

  it('shows an error message when unshare request fails', async () => {
    server.use(
      http.delete('/api/v1/workspaces/:id/share/:userId', () =>
        HttpResponse.json({ error: 'Permission denied' }, { status: 403 })
      )
    );

    const user = userEvent.setup();
    renderWithProviders(<ShareDialog {...defaultProps} />);
    await waitFor(() => expect(screen.getByText(mockCollaborator.username)).toBeInTheDocument());

    // Click the remove button and confirm
    const removeButtons = screen.getAllByRole('button');
    const removeBtn = removeButtons.find((b) => b.querySelector('svg'));
    await user.click(removeBtn!);
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Remove Collaborator' })).toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: 'Remove' }));

    await waitFor(() => expect(screen.getByText('Permission denied')).toBeInTheDocument());
  });
});
