import { screen } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import { useModeStore } from '@/store/modeStore';
import { renderWithProviders } from '@/test/utils';
import { Login } from './Login';

describe('Login', () => {
  beforeEach(() => {
    useModeStore.setState({ mode: 'team', logoutUrl: null, loading: false });
  });

  it('shows pending identity-review status from auth redirects', async () => {
    renderWithProviders(<Login isDarkMode={false} />, {
      initialEntries: ['/login?error=identity_review_pending'],
    });

    expect(
      await screen.findByText(
        'Your identity link request is pending admin approval. Try again after an admin approves it.',
      ),
    ).toBeInTheDocument();
  });

  it('shows rejected identity-review status from auth redirects', async () => {
    renderWithProviders(<Login isDarkMode={false} />, {
      initialEntries: ['/login?error=identity_review_rejected'],
    });

    expect(
      await screen.findByText(
        'Your identity link request was rejected by an admin. Contact an administrator if you believe this is a mistake.',
      ),
    ).toBeInTheDocument();
  });

  it('does not echo unknown auth redirect errors', async () => {
    renderWithProviders(<Login isDarkMode={false} />, {
      initialEntries: ['/login?error=Call%20IT%20at%201-800-555-0199'],
    });

    expect(
      await screen.findByText('Authentication failed'),
    ).toBeInTheDocument();
    expect(
      screen.queryByText('Call IT at 1-800-555-0199'),
    ).not.toBeInTheDocument();
  });

  it('shows login options when no proxy gateway is configured', async () => {
    renderWithProviders(<Login isDarkMode={false} />, {
      initialEntries: ['/login'],
    });

    expect(
      await screen.findByRole('button', { name: /sign in with oauth/i }),
    ).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Username')).toBeInTheDocument();
  });
});
