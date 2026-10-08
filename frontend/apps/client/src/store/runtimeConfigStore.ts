import { create } from 'zustand';
import { apiClient } from '@/api/client';

interface RuntimeConfigState {
  features: Record<string, boolean>;
  logoutUrl: string | null;
  loading: boolean;
  fetchConfig: () => Promise<void>;
}

const VERSION_ATTEMPTS = 3;
const VERSION_RETRY_DELAY_MS = 300;

export const useRuntimeConfigStore = create<RuntimeConfigState>()((set) => ({
  features: {},
  logoutUrl: null,
  loading: true,
  fetchConfig: async () => {
    // Retry while the backend starts up.
    for (let attempt = 1; attempt <= VERSION_ATTEMPTS; attempt++) {
      try {
        const { data } = await apiClient.get('/version');
        set({
          features: data.features || {},
          logoutUrl: data.logout_url || null,
          loading: false,
        });
        return;
      } catch {
        if (attempt < VERSION_ATTEMPTS) {
          await new Promise((resolve) =>
            setTimeout(resolve, VERSION_RETRY_DELAY_MS),
          );
        }
      }
    }
    set({
      features: {},
      logoutUrl: null,
      loading: false,
    });
  },
}));
