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
    expect(screen.getByText('Pending')).toBeInTheDocument();
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

  it('shows rejected identity reviews without actions', async () => {
    server.use(
      http.get('/api/v1/admin/federated-identity-reviews', () =>
        HttpResponse.json([
          {
            ...mockFederatedIdentityReview,
            status: 'rejected',
            reviewed_at: '2024-01-02T00:00:00Z',
          },
        ]),
      ),
    );

    renderWithProviders(<FederatedIdentityReviews />);

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
  });
});
