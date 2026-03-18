import { create } from 'zustand';

interface WorkspaceNavState {
  pendingTab: string | null;
  setPendingTab: (tab: string) => void;
  consumePendingTab: () => string | null;
}

export const useWorkspaceNavStore = create<WorkspaceNavState>((set, get) => ({
  pendingTab: null,
  setPendingTab: (tab) => set({ pendingTab: tab }),
  consumePendingTab: () => {
    const tab = get().pendingTab;
    set({ pendingTab: null });
    return tab;
  },
}));
