import { useState, useEffect } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';
import { authApi } from '@/api/auth';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { apiClient } from '@/api/client';
import { getApiBaseUrl } from '@/lib/basePath';

export const Login = () => {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
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

  // Try session-based auth (works with any authenticating proxy)
  useEffect(() => {
    if (isLocalMode) return;
    apiClient.get('/auth/session', { withCredentials: true })
      .then(({ data }) => {
        setAuth(data.token, data.user);
        navigate('/');
      })
      .catch(() => {
        // No proxy session, show login form as usual
        setSessionChecked(true);
      });
  }, [isLocalMode, setAuth, navigate]);

  // Don't render login form in local mode
  if (isLocalMode) return null;

  // Handle OAuth callback
  useEffect(() => {
    const token = searchParams.get('token');
    const oauthError = searchParams.get('error');

    if (oauthError) {
      setError('OAuth authentication failed');
      return;
    }

    if (token) {
      // Fetch user info with the token
      const fetchUser = async () => {
        try {
          setLoading(true);
          // Fetch current user info
          const response = await apiClient.get('/auth/me', {
            headers: { Authorization: `Bearer ${token}` },
          });

          setAuth(token, response.data);
          navigate('/');
        } catch {
          setError('Failed to complete OAuth login');
        } finally {
          setLoading(false);
        }
      };
      fetchUser();
    }
  }, [searchParams, setAuth, navigate]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      const response = await authApi.login({ username, password });
      setAuth(response.token, response.user);
      navigate('/');
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      setError(error.response?.data?.error || 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  // Wait for session check before showing the form
  if (!sessionChecked) return null;

  return (
    <div className="min-h-screen flex items-center justify-center bg-white">
      <div className="w-full max-w-lg">
        <div className="space-y-6 pb-8">
          <div className="flex justify-center">
            <img
              src="/nebi-logo.png"
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
            {error && (
              <div className="bg-destructive/10 text-destructive p-4 rounded-md text-sm">
                {error}
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
              type="submit"
              disabled={loading}
              className="w-full h-12 text-base font-medium"
            >
              {loading ? 'Signing in...' : 'Sign in'}
            </Button>
          </form>

          <div className="relative my-6">
            <div className="absolute inset-0 flex items-center">
              <div className="w-full border-t border-gray-300"></div>
            </div>
            <div className="relative flex justify-center text-sm">
              <span className="px-2 bg-white text-gray-500">Or continue with</span>
            </div>
          </div>

          <Button
            onClick={() => window.location.href = `${getApiBaseUrl()}/auth/oidc/login`}
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
