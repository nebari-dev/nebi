import { QueryClient } from '@tanstack/react-query';

// networkMode starts as 'always' because the app mode isn't known yet and the
// desktop app talks to the loopback backend, which stays reachable even when
// the OS reports the network as offline — the default 'online' mode would
// pause fetches/retries on offline events and wedge queries in a permanent
// 'paused' state (issue #217). Once the mode resolves, modeStore keeps
// 'always' for local mode and flips to 'online' for team mode, where the API
// is a real network hop and pausing while offline is the better behavior.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
      networkMode: 'always',
    },
    mutations: {
      networkMode: 'always',
    },
  },
});

export const setQueryNetworkMode = (networkMode: 'always' | 'online') => {
  queryClient.setDefaultOptions({
    queries: {
      ...queryClient.getDefaultOptions().queries,
      networkMode,
    },
    mutations: {
      ...queryClient.getDefaultOptions().mutations,
      networkMode,
    },
  });
};
