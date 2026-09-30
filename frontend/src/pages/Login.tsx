import { useEffect, useState } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { getBrandingLogoUrl } from '@/lib/brandingConfig';
import { getUserManager, safeReturnTo } from '@/lib/oidc';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';

type LoginProps = {
  isDarkMode: boolean;
};

export const Login = ({ isDarkMode }: LoginProps) => {
  const [searchParams] = useSearchParams();
  const location = useLocation();
  const navigate = useNavigate();
  const [signingIn, setSigningIn] = useState(false);
  const [startFailed, setStartFailed] = useState(false);

  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const isOidc = useAuthStore((s) => s.isOidc());
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated());

  // Where to go after signing in: PrivateRoute passes the blocked location as
  // router state; a full-page redirect after a rejected token uses ?returnTo=.
  const from = (location.state as { from?: unknown } | null)?.from;
  const returnTo =
    safeReturnTo(from) ??
    safeReturnTo(searchParams.get('returnTo')) ??
    '/projects';

  // Nothing to sign in to in local mode or when the server has auth disabled,
  // and nothing to do when a session already exists.
  const skipLogin = isLocalMode || !isOidc || isAuthenticated;

  useEffect(() => {
    if (skipLogin) {
      navigate(isLocalMode || !isOidc ? '/projects' : returnTo, {
        replace: true,
      });
    }
  }, [skipLogin, isLocalMode, isOidc, returnTo, navigate]);

  if (skipLogin) return null;

  const handleSignIn = async () => {
    setStartFailed(false);
    setSigningIn(true);
    try {
      const manager = getUserManager();
      if (!manager) throw new Error('OIDC is not configured');
      // Navigates away to the identity provider on success.
      await manager.signinRedirect({ state: { returnTo } });
    } catch {
      setStartFailed(true);
      setSigningIn(false);
    }
  };

  const errorMessage = startFailed
    ? 'Could not reach the sign-in provider. Please try again.'
    : searchParams.get('error')
      ? 'Sign in failed. Please try again.'
      : '';

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
            Project Management System
          </p>
        </div>
        <div className="space-y-4 px-8 pb-8">
          {errorMessage && (
            <div
              role="alert"
              className="rounded-md bg-destructive/10 p-4 text-sm text-destructive"
            >
              {errorMessage}
            </div>
          )}

          <Button
            onClick={handleSignIn}
            disabled={signingIn}
            className="w-full h-12 text-base font-medium"
          >
            {signingIn ? 'Redirecting...' : 'Sign in'}
          </Button>
        </div>
      </div>
    </div>
  );
};
