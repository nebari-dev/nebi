import { QueryClient } from '@tanstack/react-query';

// The server API requires a network connection.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
      networkMode: 'online',
    },
    mutations: {
      networkMode: 'online',
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
