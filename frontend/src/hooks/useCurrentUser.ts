import { useQuery } from '@tanstack/react-query';
import { useEffect } from 'react';
import { authApi } from '@/api/auth';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';

// Loads the Nebi user behind the current credentials into authStore.user.
// Local mode has no signed-in user, so it skips the request.
export const useCurrentUser = () => {
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const setUser = useAuthStore((s) => s.setUser);
  const query = useQuery({
    queryKey: ['auth', 'me'],
    queryFn: authApi.me,
    enabled: !isLocalMode,
    staleTime: Number.POSITIVE_INFINITY,
  });

  useEffect(() => {
    if (query.data) setUser(query.data);
  }, [query.data, setUser]);

  return query;
};
