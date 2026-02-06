import { create } from 'zustand';

interface RemoteState {
  serverUrl: string | null;
  username: string | null;
  connected: boolean;
  setConnection: (url: string, username: string) => void;
  clearConnection: () => void;
}

export const useRemoteStore = create<RemoteState>()((set) => ({
  serverUrl: null,
  username: null,
  connected: false,
  setConnection: (url: string, username: string) =>
    set({ serverUrl: url, username, connected: true }),
  clearConnection: () =>
    set({ serverUrl: null, username: null, connected: false }),
}));
