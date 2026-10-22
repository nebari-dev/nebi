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

// The Wails desktop app injects `window.runtime`; its loopback backend stays
// reachable even when the OS reports the network as offline.
const isDesktopApp = () => typeof window !== 'undefined' && !!window.runtime;

const VERSION_ATTEMPTS = 3;
const VERSION_RETRY_DELAY_MS = 300;

export const useModeStore = create<ModeState>()((set, get) => ({
  mode: null,
  features: {},
  logoutUrl: null,
  loading: true,
  fetchMode: async () => {
    // The desktop app starts its embedded server asynchronously, so the first
    // /version call can race it — retry before falling back to team mode.
    for (let attempt = 1; attempt <= VERSION_ATTEMPTS; attempt++) {
      try {
        const { data } = await apiClient.get('/version');
        // Local (desktop) mode keeps the 'always' networkMode the client
        // starts with — its loopback API is reachable even when the OS
        // reports the network as offline (issue #217). Team mode talks to a
        // real server, so switch to the default 'online' mode, which pauses
        // while offline.
        setQueryNetworkMode(data.mode === 'local' ? 'always' : 'online');
        set({
          mode: data.mode,
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
    // When /version never answers, fall back by surface: the desktop app is
    // always local mode, so resolving to team there would strand it on the
    // Login page over a transient API hiccup (issue #530) — everywhere else
    // default to team. Never downgrade networkMode in the desktop app —
    // pinning 'online' there would wedge every loopback query in 'paused'
    // while the OS reports offline, which is exactly the issue #217 scenario.
    const desktop = isDesktopApp();
    setQueryNetworkMode(desktop ? 'always' : 'online');
    set({
      mode: desktop ? 'local' : 'team',
      features: {},
      logoutUrl: null,
      loading: false,
    });
  },
  isLocalMode: () => get().mode === 'local',
}));
