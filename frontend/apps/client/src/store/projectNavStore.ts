import { create } from 'zustand';

interface ProjectNavState {
  pendingTab: string | null;
  setPendingTab: (tab: string) => void;
  consumePendingTab: () => string | null;
}

export const useProjectNavStore = create<ProjectNavState>((set, get) => ({
  pendingTab: null,
  setPendingTab: (tab) => set({ pendingTab: tab }),
  consumePendingTab: () => {
    const tab = get().pendingTab;
    set({ pendingTab: null });
    return tab;
  },
}));
