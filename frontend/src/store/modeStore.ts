import { create } from 'zustand';
import { apiClient } from '@/api/client';
import { setQueryNetworkMode } from '@/lib/queryClient';

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
      // Local (desktop) mode keeps the 'always' networkMode the client starts
      // with — its loopback API is reachable even when the OS reports the
      // network as offline (issue #217). Team mode talks to a real server, so
      // switch to the default 'online' mode, which pauses while offline.
      setQueryNetworkMode(data.mode === 'local' ? 'always' : 'online');
      set({
        mode: data.mode,
        features: data.features || {},
        logoutUrl: data.logout_url || null,
        loading: false,
      });
    } catch {
      // Default to team mode on error
      setQueryNetworkMode('online');
      set({ mode: 'team', features: {}, logoutUrl: null, loading: false });
    }
  },
  isLocalMode: () => get().mode === 'local',
}));
