import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';
import {
  mockFederatedIdentity,
  mockFederatedIdentityReview,
  server,
} from '@/test/handlers';
import { renderWithProviders } from '@/test/utils';
import { FederatedIdentityReviews } from './FederatedIdentityReviews';

describe('FederatedIdentityReviews', () => {
  beforeEach(() => {
    useModeStore.setState({ mode: 'team', loading: false });
    useViewModeStore.setState({ viewMode: 'local' });
    server.use(
      http.get('/api/v1/remote/server', () =>
        HttpResponse.json({ status: 'disconnected', url: '', username: '' }),
      ),
    );
  });

  it('shows pending identity review details', async () => {
    renderWithProviders(<FederatedIdentityReviews />);

    expect(await screen.findByText('Identity Reviews')).toBeInTheDocument();
    expect(screen.getAllByText('Pending')).toHaveLength(2);
    expect(screen.getByText('https://issuer.example.com')).toBeInTheDocument();
    expect(screen.getByText('sub: subject-1')).toBeInTheDocument();
    expect(screen.getByText('Email verified')).toBeInTheDocument();
  });

  it('approves a pending identity review', async () => {
    const user = userEvent.setup();
    let approvedReviewID = '';
    server.use(
      http.post(
        '/api/v1/admin/federated-identity-reviews/:id/approve',
        ({ params }) => {
          approvedReviewID = params.id as string;
          return HttpResponse.json(mockFederatedIdentity, { status: 201 });
        },
      ),
    );

    renderWithProviders(<FederatedIdentityReviews />);

    await screen.findByText(mockFederatedIdentityReview.issuer);
    await user.click(
      screen.getByRole('button', {
        name: /approve identity review for testuser/i,
      }),
    );
    await user.click(screen.getByRole('button', { name: 'Approve Link' }));

    await waitFor(() => expect(approvedReviewID).toBe('review-1'));
  });

  it('rejects a pending identity review', async () => {
    const user = userEvent.setup();
    let rejectedReviewID = '';
    server.use(
      http.post(
        '/api/v1/admin/federated-identity-reviews/:id/reject',
        ({ params }) => {
          rejectedReviewID = params.id as string;
          return new HttpResponse(null, { status: 204 });
        },
      ),
    );

    renderWithProviders(<FederatedIdentityReviews />);

    await screen.findByText(mockFederatedIdentityReview.issuer);
    await user.click(
      screen.getByRole('button', {
        name: /reject identity review for testuser/i,
      }),
    );
    await user.click(screen.getByRole('button', { name: 'Reject Request' }));

    await waitFor(() => expect(rejectedReviewID).toBe('review-1'));
  });

  it('shows split username and email collisions as ambiguous', async () => {
    server.use(
      http.get('/api/v1/admin/federated-identity-reviews', () =>
        HttpResponse.json([
          {
            ...mockFederatedIdentityReview,
            collision_field: 'username,email',
            collision_username_user_id: 'user-alice',
            collision_username_user: {
              id: 'user-alice',
              username: 'alice',
              email: 'alice@example.com',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
            collision_email_user_id: 'user-bob',
            collision_email_user: {
              id: 'user-bob',
              username: 'bob',
              email: 'bob@example.com',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
            username: 'alice',
            email: 'bob@example.com',
          },
        ]),
      ),
    );

    renderWithProviders(<FederatedIdentityReviews />);

    expect(await screen.findByText('multiple users')).toBeInTheDocument();
    expect(screen.getByText('Username match')).toBeInTheDocument();
    expect(screen.getByText('Email match')).toBeInTheDocument();
    expect(screen.getByText('alice@example.com')).toBeInTheDocument();
    expect(screen.getAllByText('bob@example.com')).toHaveLength(2);
    expect(
      screen.getByRole('button', {
        name: /approve identity review for alice/i,
      }),
    ).toBeDisabled();
  });

  it('shows rejected identity reviews without actions', async () => {
    const user = userEvent.setup();
    server.use(
      http.get('/api/v1/admin/federated-identity-reviews', ({ request }) => {
        const url = new URL(request.url);
        if (url.searchParams.get('status') !== 'rejected') {
          return HttpResponse.json([]);
        }
        return HttpResponse.json([
          {
            ...mockFederatedIdentityReview,
            status: 'rejected',
            reviewed_at: '2024-01-02T00:00:00Z',
          },
        ]);
      }),
    );

    renderWithProviders(<FederatedIdentityReviews />);

    await user.click(await screen.findByRole('tab', { name: 'Rejected' }));

    expect(await screen.findAllByText('Rejected')).toHaveLength(2);
    expect(
      screen.queryByRole('button', {
        name: /approve identity review for testuser/i,
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', {
        name: /reject identity review for testuser/i,
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole('button', {
        name: /discard identity review for testuser/i,
      }),
    ).toBeInTheDocument();
  });

  it('discards a rejected identity review', async () => {
    const user = userEvent.setup();
    let discardedReviewID = '';
    server.use(
      http.get('/api/v1/admin/federated-identity-reviews', ({ request }) => {
        const url = new URL(request.url);
        if (url.searchParams.get('status') !== 'rejected') {
          return HttpResponse.json([]);
        }
        return HttpResponse.json([
          {
            ...mockFederatedIdentityReview,
            status: 'rejected',
            reviewed_at: '2024-01-02T00:00:00Z',
          },
        ]);
      }),
      http.delete(
        '/api/v1/admin/federated-identity-reviews/:id',
        ({ params }) => {
          discardedReviewID = params.id as string;
          return new HttpResponse(null, { status: 204 });
        },
      ),
    );

    renderWithProviders(<FederatedIdentityReviews />);

    await user.click(await screen.findByRole('tab', { name: 'Rejected' }));
    await screen.findByText(mockFederatedIdentityReview.issuer);
    await user.click(
      screen.getByRole('button', {
        name: /discard identity review for testuser/i,
      }),
    );
    await user.click(screen.getByRole('button', { name: 'Discard Review' }));

    await waitFor(() => expect(discardedReviewID).toBe('review-1'));
  });

  it('shows the remote unreachable banner when remote reviews fail', async () => {
    useModeStore.setState({ mode: 'local', loading: false });
    useViewModeStore.setState({ viewMode: 'remote' });
    server.use(
      http.get('/api/v1/remote/server', () =>
        HttpResponse.json({
          status: 'connected',
          url: 'https://remote.example.com',
          username: 'admin',
        }),
      ),
      http.get('/api/v1/remote/admin/federated-identity-reviews', () =>
        HttpResponse.json({ error: 'Remote error' }, { status: 502 }),
      ),
    );

    renderWithProviders(<FederatedIdentityReviews />);

    expect(
      await screen.findByText(/can't reach the remote server/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByText('No pending identity reviews'),
    ).not.toBeInTheDocument();
  });
});
