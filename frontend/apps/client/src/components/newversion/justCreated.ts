// Owner: U3 (newversion). The prototype's "version just created" cue: the
// new version's row in the rail lights up, then fades (JustCreated).
import { useEffect, useState } from 'react';

/** How long after creation a newly shown row still lights up. */
const LIGHT_WINDOW_MS = 2000;
const LIT_MS = 600;
const FADE_MS = 1200;

let last: { projectId: string; version: number; at: number } | null = null;

/** Records that `version` of `projectId` was just created. */
export function markJustCreated(projectId: string, version: number) {
  last = { projectId, version, at: Date.now() };
}

/** Test helper: forget the last created version. */
export function resetJustCreated() {
  last = null;
}

export type JustCreatedState = 'lit' | 'fading' | null;

/**
 * 'lit' when this version's row first renders within a moment of its
 * creation, then 'fading' while the highlight eases out, then null.
 */
export function useJustCreated(
  projectId: string | undefined,
  version: number,
): JustCreatedState {
  const [state, setState] = useState<JustCreatedState>(() =>
    projectId &&
    last?.projectId === projectId &&
    last.version === version &&
    Date.now() - last.at < LIGHT_WINDOW_MS
      ? 'lit'
      : null,
  );
  useEffect(() => {
    if (!state) return;
    const t = window.setTimeout(
      () => setState(state === 'lit' ? 'fading' : null),
      state === 'lit' ? LIT_MS : FADE_MS,
    );
    return () => window.clearTimeout(t);
  }, [state]);
  return state;
}
