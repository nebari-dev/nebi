// Owner: U3 (newversion). Blocks leaving a page with unsaved changes.
//
// The app uses <BrowserRouter> (no data router, so no useBlocker). The
// guard covers what a user can do from the page:
//   - links anywhere in the app (header nav, breadcrumbs): a capture-phase
//     click listener stops the navigation and asks first;
//   - in-page buttons (Cancel): call `leave(path)`;
//   - reload / closing the tab: the browser's own beforeunload prompt.
import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { getBasePath } from '@/lib/basePath';

/** App path of an internal link, or null when the link leaves the app. */
function appPathOf(anchor: HTMLAnchorElement): string | null {
  if (anchor.target && anchor.target !== '_self') return null;
  if (anchor.hasAttribute('download')) return null;
  const url = new URL(anchor.href, window.location.href);
  if (url.origin !== window.location.origin) return null;
  const base = getBasePath();
  let path = url.pathname;
  if (base) {
    if (!path.startsWith(base)) return null;
    path = path.slice(base.length) || '/';
  }
  return `${path}${url.search}${url.hash}`;
}

export function useUnsavedChangesGuard(dirty: boolean) {
  const navigate = useNavigate();
  const [pendingPath, setPendingPath] = useState<string | null>(null);
  const bypass = useRef(false);

  useEffect(() => {
    if (!dirty) return;
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      if (bypass.current) return;
      e.preventDefault();
    };
    const onClick = (e: MouseEvent) => {
      if (bypass.current || e.defaultPrevented || e.button !== 0) return;
      if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      const anchor = (e.target as Element | null)?.closest?.('a[href]');
      if (!(anchor instanceof HTMLAnchorElement)) return;
      const path = appPathOf(anchor);
      if (path == null) return;
      e.preventDefault();
      e.stopPropagation();
      setPendingPath(path);
    };
    window.addEventListener('beforeunload', onBeforeUnload);
    document.addEventListener('click', onClick, true);
    return () => {
      window.removeEventListener('beforeunload', onBeforeUnload);
      document.removeEventListener('click', onClick, true);
    };
  }, [dirty]);

  /** Navigate without asking (after saving). */
  const navigateAway = useCallback(
    (path: string, opts?: { replace?: boolean }) => {
      bypass.current = true;
      setPendingPath(null);
      navigate(path, opts);
    },
    [navigate],
  );

  /** Leave to `path`, asking first when there are unsaved changes. */
  const leave = useCallback(
    (path: string) => {
      if (dirty) setPendingPath(path);
      else navigateAway(path);
    },
    [dirty, navigateAway],
  );

  return {
    /** Where the user tried to go while the discard dialog is open. */
    pendingPath,
    leave,
    navigateAway,
    /** Discard the changes and continue to the pending path. */
    discard: () => pendingPath != null && navigateAway(pendingPath),
    /** Stay on the page. */
    stay: () => setPendingPath(null),
  };
}
