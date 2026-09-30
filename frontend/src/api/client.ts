import axios, { type InternalAxiosRequestConfig } from 'axios';
import { getApiBaseUrl, getBasePath } from '@/lib/basePath';
import {
  getAccessToken,
  getUserManager,
  renewAccessToken,
  safeReturnTo,
} from '@/lib/oidc';
import { queryClient } from '@/lib/queryClient';
import { useModeStore } from '@/store/modeStore';

const API_BASE_URL = import.meta.env.VITE_API_URL || getApiBaseUrl();

export const apiClient = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

type RetriableRequestConfig = InternalAxiosRequestConfig & {
  _authRetried?: boolean;
};

// The UserManager only exists once a team server reports OIDC auth; local
// mode and auth-disabled team servers send no credentials at all.
const usesOidc = () =>
  useModeStore.getState().mode !== 'local' && getUserManager() !== null;

const redirectToLogin = () => {
  const basePath = getBasePath();
  const { pathname, search } = window.location;
  const current = pathname.startsWith(basePath)
    ? pathname.slice(basePath.length)
    : pathname;
  if (current.startsWith('/login') || current.startsWith('/auth/callback')) {
    return;
  }
  const returnTo = safeReturnTo(current + search);
  const query = returnTo ? `?returnTo=${encodeURIComponent(returnTo)}` : '';
  window.location.href = `${basePath}/login${query}`;
};

// Request interceptor to attach the identity provider's access token
apiClient.interceptors.request.use(
  async (config) => {
    if (usesOidc()) {
      const token = await getAccessToken();
      if (token) {
        config.headers.Authorization = `Bearer ${token}`;
      }
    }
    return config;
  },
  (error) => Promise.reject(error),
);

// Response interceptor for error handling
apiClient.interceptors.response.use(
  (response) => response,
  async (error) => {
    if (error.response?.status !== 401 || !usesOidc()) {
      return Promise.reject(error);
    }

    // The access token was rejected: renew it once and replay the request.
    const config = error.config as RetriableRequestConfig | undefined;
    if (config && !config._authRetried) {
      config._authRetried = true;
      const token = await renewAccessToken();
      if (token) {
        config.headers.Authorization = `Bearer ${token}`;
        return apiClient.request(config);
      }
    }

    // Renewal failed: drop the session and its cached data, then sign in again.
    await getUserManager()
      ?.removeUser()
      .catch(() => undefined);
    queryClient.clear();
    redirectToLogin();
    return Promise.reject(error);
  },
);
