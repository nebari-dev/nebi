// Owner: U2 (detail). Guided project tour state. The "seen" flag is a
// per-viewer convenience kept in localStorage (wrapped in try/catch: the
// tour still works when storage is blocked, it just asks again).
import { create } from 'zustand';

export const TOUR_SEEN_KEY = 'nebi.projectTour.seen';

function readSeen(): boolean {
  try {
    return window.localStorage.getItem(TOUR_SEEN_KEY) === '1';
  } catch {
    return false;
  }
}

function writeSeen(): void {
  try {
    window.localStorage.setItem(TOUR_SEEN_KEY, '1');
  } catch {
    // Storage unavailable: the prompt comes back next time.
  }
}

interface TourState {
  /** The tour is running. */
  active: boolean;
  /** The viewer started, skipped or finished the tour once. */
  seen: boolean;
  start: () => void;
  stop: () => void;
  dismissPrompt: () => void;
  /** Re-reads the stored flag (tests, storage changes). */
  reload: () => void;
}

export const useTourStore = create<TourState>((set) => ({
  active: false,
  seen: readSeen(),
  start: () => {
    writeSeen();
    set({ active: true, seen: true });
  },
  stop: () => {
    writeSeen();
    set({ active: false, seen: true });
  },
  dismissPrompt: () => {
    writeSeen();
    set({ seen: true });
  },
  reload: () => set({ active: false, seen: readSeen() }),
}));
