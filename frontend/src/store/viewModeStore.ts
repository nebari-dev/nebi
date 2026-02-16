import { create } from 'zustand';
import { persist } from 'zustand/middleware';

type ViewMode = 'local' | 'remote';

interface ViewModeState {
  viewMode: ViewMode;
  setViewMode: (mode: ViewMode) => void;
}

export const useViewModeStore = create<ViewModeState>()(
  persist(
    (set) => ({
      viewMode: 'local',
      setViewMode: (mode) => set({ viewMode: mode }),
    }),
    { name: 'nebi-view-mode' }
  )
);
