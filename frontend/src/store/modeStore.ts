import { create } from 'zustand';
import { apiClient } from '@/api/client';

interface ModeState {
  mode: 'local' | 'team' | null;
  features: Record<string, boolean>;
  logoutUrl: string | null;
  loading: boolean;
  fetchMode: () => Promise<void>;
  isLocalMode: () => boolean;
}

export const useModeStore = create<ModeState>()((set, get) => ({
  mode: null,
  features: {},
  logoutUrl: null,
  loading: true,
  fetchMode: async () => {
    try {
      const { data } = await apiClient.get('/version');
      set({ mode: data.mode, features: data.features || {}, logoutUrl: data.logout_url || null, loading: false });
    } catch {
      // Default to team mode on error
      set({ mode: 'team', features: {}, logoutUrl: null, loading: false });
    }
  },
  isLocalMode: () => get().mode === 'local',
}));
