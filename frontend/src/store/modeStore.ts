import { create } from 'zustand';
import { apiClient } from '@/api/client';

interface ModeState {
  mode: 'local' | 'team' | null;
  features: Record<string, boolean>;
  loading: boolean;
  fetchMode: () => Promise<void>;
  isLocalMode: () => boolean;
}

export const useModeStore = create<ModeState>()((set, get) => ({
  mode: null,
  features: {},
  loading: true,
  fetchMode: async () => {
    try {
      const { data } = await apiClient.get('/version');
      set({ mode: data.mode, features: data.features || {}, loading: false });
    } catch {
      // Default to team mode on error
      set({ mode: 'team', features: {}, loading: false });
    }
  },
  isLocalMode: () => get().mode === 'local',
}));
