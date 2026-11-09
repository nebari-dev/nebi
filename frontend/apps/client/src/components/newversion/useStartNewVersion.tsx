// Owner: U3 (newversion). Entry point to the new version page for other
// pages (the project detail page, U2): starting from the latest version
// goes straight to the page; starting from an older version asks first
// ("Start a new version from version N?").
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import type { VersionItem } from '@/types/contracts';
import { newVersionPath } from './changes';
import { OlderBaseConfirmDialog } from './OlderBaseConfirmDialog';

/**
 * Returns `start(n)` and the confirmation `dialog` element (render it once
 * anywhere in the page). `versions` is the version rail; server-only
 * versions are ignored (pull them first).
 */
export function useStartNewVersion(
  projectId: string,
  versions: VersionItem[] | undefined,
) {
  const navigate = useNavigate();
  const [pending, setPending] = useState<number | null>(null);
  const local = (versions ?? [])
    .filter((v) => !v.server_only)
    .map((v) => v.number);
  const latestVersion = local.length ? Math.max(...local) : 0;
  const newerVersions = (n: number) =>
    local.filter((x) => x > n).sort((a, b) => a - b);

  const go = (n: number) => {
    setPending(null);
    navigate(newVersionPath(projectId, n));
  };
  const start = (n: number) => {
    if (newerVersions(n).length > 0) setPending(n);
    else go(n);
  };

  const dialog = (
    <OlderBaseConfirmDialog
      open={pending != null}
      onOpenChange={(open) => !open && setPending(null)}
      baseVersion={pending ?? 0}
      latestVersion={latestVersion}
      newerVersions={pending != null ? newerVersions(pending) : []}
      onStart={go}
    />
  );
  return { start, latestVersion, dialog };
}
