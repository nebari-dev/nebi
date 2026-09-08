import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { authApi } from '@/api/auth';
import { apiClient } from '@/api/client';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { getApiBaseUrl, getBasePath } from '@/lib/basePath';
import { getBrandingLogoUrl } from '@/lib/brandingConfig';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';

type LoginProps = {
  isDarkMode: boolean;
};

const authErrorMessage = (error: string | null) => {
  switch (error) {
    case 'identity_review_pending':
      return 'Your identity link request is pending admin approval. Try again after an admin approves it.';
    case 'identity_review_rejected':
      return 'Your identity link request was rejected by an admin. Contact an administrator if you believe this is a mistake.';
    case 'invalid credentials':
      return 'Invalid credentials';
    case 'code_exchange_failed':
      return 'Failed to complete login';
    case 'login_failed':
      return 'Login failed';
    default:
      return 'Authentication failed';
  }
};

const authErrorTone = (error: string | null) =>
  error === 'identity_review_pending' ? 'warning' : 'destructive';

export const Login = ({ isDarkMode }: LoginProps) => {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [authError, setAuthError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [sessionChecked, setSessionChecked] = useState(false);
  const [searchParams] = useSearchParams();

  const setAuth = useAuthStore((state) => state.setAuth);
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const navigate = useNavigate();

  // In local mode, redirect straight to workspaces
  useEffect(() => {
    if (isLocalMode) {
      navigate('/workspaces');
    }
  }, [isLocalMode, navigate]);

  // Auto-login via OIDC gateway proxy (RFC 6749 §4.1 authorization code pattern):
  // 1. Redirect to /auth/session (outside /api/, so gateway preserves cookies)
  // 2. Backend reads IdToken cookie → generates single-use code → redirects to /login?code=xxx
  // 3. Frontend exchanges code for JWT via POST /api/v1/auth/code/exchange
  useEffect(() => {
    if (isLocalMode) return;
    if (searchParams.get('code') || searchParams.get('error')) return;
    if (sessionStorage.getItem('nebi_logout')) {
      sessionStorage.removeItem('nebi_logout');
      setSessionChecked(true);
      return;
    }
    const logoutUrl = useModeStore.getState().logoutUrl;
    if (logoutUrl) {
      // Use the non-API redirect endpoint so gateway proxies preserve OIDC
      // cookies before Nebi exchanges the resulting single-use code.
      window.location.href = `${getBasePath()}/auth/session`;
      return;
    }
    // No gateway detected — show login form
    setSessionChecked(true);
  }, [isLocalMode, searchParams]);

  // Exchange single-use authorization code for JWT.
  // Used by both gateway auto-login (/auth/session) and direct OIDC callback
  // (/api/v1/auth/oidc/callback). Both redirect here with ?code=<single-use-code>.
  useEffect(() => {
    if (isLocalMode) return;

    const code = searchParams.get('code');
    const oauthError = searchParams.get('error');

    if (oauthError) {
      setAuthError(oauthError);
      setSessionChecked(true);
      return;
    }

    if (code) {
      const exchangeCode = async () => {
        try {
          setLoading(true);
          const { data } = await apiClient.post('/auth/code/exchange', {
            code,
          });
          setAuth(data.token, data.user);
          navigate('/');
        } catch {
          setAuthError('code_exchange_failed');
          setSessionChecked(true);
        } finally {
          setLoading(false);
        }
      };
      exchangeCode();
    }
  }, [isLocalMode, searchParams, setAuth, navigate]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setAuthError(null);
    setLoading(true);

    try {
      const response = await authApi.login({ username, password });
      setAuth(response.token, response.user);
      navigate('/');
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      const authError = error.response?.data?.error;
      setAuthError(authError || 'login_failed');
    } finally {
      setLoading(false);
    }
  };

  // Don't render login form in local mode
  if (isLocalMode) return null;

  // Wait for session check before showing the form
  if (!sessionChecked) return null;

  const errorMessage = authError ? authErrorMessage(authError) : '';
  const errorTone = authErrorTone(authError);

  return (
    <div className="min-h-screen flex items-center justify-center bg-canvas">
      <div className="w-full max-w-lg">
        <div className="space-y-6 pb-8">
          <div className="flex justify-center">
            <img
              src={getBrandingLogoUrl(isDarkMode)}
              alt="Nebi Logo"
              className="h-24 w-auto"
            />
          </div>
          <p className="text-center text-muted-foreground text-base">
            Workspace Management System
          </p>
        </div>
        <div className="px-8 pb-8">
          <form onSubmit={handleSubmit} className="space-y-4">
            {errorMessage && (
              <div
                className={
                  errorTone === 'warning'
                    ? 'rounded-md border border-amber-300 bg-amber-100 p-4 text-sm text-amber-800'
                    : 'rounded-md bg-destructive/10 p-4 text-sm text-destructive'
                }
              >
                {errorMessage}
              </div>
            )}

            <Input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="Username"
              required
              className="h-12 text-base"
            />

            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Password"
              required
              className="h-12 text-base"
            />

            <Button
              render={<button type="submit" />}
              disabled={loading}
              className="w-full h-12 text-base font-medium"
            >
              {loading ? 'Signing in...' : 'Sign in'}
            </Button>
          </form>

          <div className="relative my-6">
            <div className="absolute inset-0 flex items-center">
              <div className="w-full border-t border-border"></div>
            </div>
            <div className="relative flex justify-center text-sm">
              <span className="px-2 bg-canvas text-muted-foreground">
                Or continue with
              </span>
            </div>
          </div>

          <Button
            onClick={() =>
              (window.location.href = `${getApiBaseUrl()}/auth/oidc/login`)
            }
            variant="outline"
            className="w-full h-12 text-base font-medium"
          >
            Sign in with OAuth
          </Button>
        </div>
      </div>
    </div>
  );
};
