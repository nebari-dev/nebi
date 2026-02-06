import { create } from 'zustand';
import { apiClient } from '@/api/client';

interface Features {
  authentication: boolean;
  userManagement: boolean;
  auditLogs: boolean;
}

interface ModeState {
  mode: 'local' | 'team' | 'loading';
  features: Features;
  fetchMode: () => Promise<void>;
  isLocalMode: () => boolean;
}

export const useModeStore = create<ModeState>()((set, get) => ({
  mode: 'loading',
  features: {
    authentication: true,
    userManagement: true,
    auditLogs: true,
  },
  fetchMode: async () => {
    try {
      const response = await apiClient.get('/version');
      const { mode, features } = response.data;
      set({
        mode: mode === 'local' ? 'local' : 'team',
        features: features ?? {
          authentication: mode !== 'local',
          userManagement: mode !== 'local',
          auditLogs: mode !== 'local',
        },
      });
    } catch {
      // Default to team mode if version endpoint fails
      set({ mode: 'team' });
    }
  },
  isLocalMode: () => get().mode === 'local',
}));
