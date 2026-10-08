import { QueryClient } from '@tanstack/react-query';

// The client talks to loopback, which remains reachable while the OS is offline.
// Remote-view requests also go through this local backend, so keep 'always'
// there too. This allows requests to run; it does not guarantee remote-server
// reachability. Remote query error handling and retries handle outages/recovery.
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
