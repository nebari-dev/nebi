import { Loader2 } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { getUserManager, safeReturnTo } from '@/lib/oidc';

// Redirect URI for the OIDC authorization code flow: exchanges the code (with
// the PKCE verifier kept by oidc-client-ts) for tokens, then returns the user
// to where they were headed.
export const AuthCallback = () => {
  const navigate = useNavigate();
  // The code can only be redeemed once, and StrictMode runs effects twice.
  const handled = useRef(false);

  useEffect(() => {
    if (handled.current) return;
    handled.current = true;

    const manager = getUserManager();
    if (!manager) {
      // Local mode or auth disabled: there is nothing to complete.
      navigate('/', { replace: true });
      return;
    }

    manager
      .signinRedirectCallback()
      .then((user) => {
        const state = user.state as { returnTo?: unknown } | undefined;
        navigate(safeReturnTo(state?.returnTo) ?? '/', { replace: true });
      })
      .catch(() => {
        navigate('/login?error=login_failed', { replace: true });
      });
  }, [navigate]);

  return (
    <div className="flex items-center justify-center h-screen">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  );
};
