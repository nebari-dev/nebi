// Owner: U2 (detail). The "In sync" moment after a push (prototype): the
// laptop icon of "Newest here" and the server icon of "Newest on server"
// slide into the "In sync" marker, then its colour and label fade in.
import { type RefObject, useLayoutEffect, useRef } from 'react';

const MOVE_MS = 240;
const FADE_MS = 200;
const REDUCED_MS = 120;
const EASE_OUT = 'cubic-bezier(0.25, 1, 0.5, 1)';

interface IconRects {
  laptop: DOMRect | null;
  server: DOMRect | null;
}

const iconRect = (root: HTMLElement, marker: string): DOMRect | null =>
  root.querySelector(`[data-sync="${marker}"] svg`)?.getBoundingClientRect() ??
  null;

const canAnimate = (el: Element): boolean =>
  typeof (el as HTMLElement).animate === 'function';

function prefersReducedMotion(): boolean {
  try {
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  } catch {
    return false;
  }
}

function playInSync(marker: HTMLElement, from: IconRects): void {
  if (!canAnimate(marker)) return;
  if (prefersReducedMotion()) {
    marker.animate([{ opacity: 0 }, { opacity: 1 }], {
      duration: REDUCED_MS,
      easing: 'linear',
    });
    return;
  }
  for (const part of ['laptop', 'server'] as const) {
    const icon = marker.querySelector(`[data-sync-part="${part}"]`);
    const start = from[part];
    if (!icon || !start || !canAnimate(icon)) continue;
    const end = icon.getBoundingClientRect();
    const dx = start.left - end.left;
    const dy = start.top - end.top;
    if (Math.abs(dx) < 1 && Math.abs(dy) < 1) continue;
    icon.animate(
      [{ transform: `translate(${dx}px, ${dy}px)` }, { transform: 'none' }],
      { duration: MOVE_MS, easing: EASE_OUT },
    );
  }
  const css = getComputedStyle(document.documentElement);
  const fromColor = css.getPropertyValue('--muted-foreground-strong').trim();
  const toColor = css.getPropertyValue('--success-foreground').trim();
  const timing: KeyframeAnimationOptions = {
    delay: MOVE_MS,
    duration: FADE_MS,
    easing: 'ease-out',
    fill: 'backwards',
  };
  if (fromColor && toColor) {
    marker.animate([{ color: fromColor }, { color: toColor }], timing);
  }
  const label = marker.querySelector('[data-sync-part="label"]');
  if (label && canAnimate(label)) {
    label.animate([{ opacity: 0 }, { opacity: 1 }], timing);
  }
}

/**
 * Plays the "In sync" animation in the rail when the project becomes in
 * sync (e.g. right after a push). Remembers where the "Newest here" and
 * "Newest on server" icons were on the previous render.
 */
export function useInSyncAnimation(
  listRef: RefObject<HTMLElement | null>,
  inSync: boolean,
): void {
  const icons = useRef<IconRects>({ laptop: null, server: null });
  const wasInSync = useRef(inSync);
  // Runs after every render: the icon positions of the last layout are
  // where the animation starts from.
  useLayoutEffect(() => {
    const root = listRef.current;
    if (!root) return;
    if (inSync && !wasInSync.current) {
      const marker = root.querySelector<HTMLElement>('[data-sync="in_sync"]');
      if (marker) playInSync(marker, icons.current);
    }
    wasInSync.current = inSync;
    icons.current = {
      laptop: iconRect(root, 'newest_here'),
      server: iconRect(root, 'newest_on_server'),
    };
  });
}

/**
 * Versions that appeared in the rail while it was showing (e.g. the one
 * just created), keyed like the rail rows. The first list the rail shows
 * marks nothing.
 */
export function useNewVersionKeys(keys: string[]): Set<string> {
  const seen = useRef<Set<string> | null>(null);
  const fresh = new Set<string>();
  if (seen.current) {
    for (const k of keys) if (!seen.current.has(k)) fresh.add(k);
  }
  const joined = keys.join(',');
  // biome-ignore lint/correctness/useExhaustiveDependencies: joined stands for keys
  useLayoutEffect(() => {
    if (keys.length === 0 && !seen.current) return;
    seen.current = new Set(keys);
  }, [joined]);
  return fresh;
}
