import { screen } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { useModeStore } from '@/store/modeStore';
import { server } from '@/test/handlers';
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

  it('shows login options when OIDC is configured without a proxy session', async () => {
    useModeStore.setState({
      mode: 'team',
      logoutUrl: '/logout',
      loading: false,
    });
    server.use(
      http.get('/api/v1/auth/session', () =>
        HttpResponse.json({ error: 'no proxy session' }, { status: 401 }),
      ),
    );

    renderWithProviders(<Login isDarkMode={false} />, {
      initialEntries: ['/login'],
    });

    expect(
      await screen.findByRole('button', { name: /sign in with oauth/i }),
    ).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Username')).toBeInTheDocument();
  });
});
