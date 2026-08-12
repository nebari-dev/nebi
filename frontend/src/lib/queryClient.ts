import { QueryClient } from '@tanstack/react-query';

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
      // The API lives on the local backend (loopback in desktop mode), which
      // stays reachable even when the OS reports the network as offline. The
      // default 'online' mode pauses fetches/retries on offline events,
      // wedging queries in a permanent 'paused' state (issue #217).
      networkMode: 'always',
    },
    mutations: {
      networkMode: 'always',
    },
  },
});
