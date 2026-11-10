// Owner: U5 (remote). Which versions "Push to server" sends.
import type { ProjectListItem, VersionItem } from '@/types/contracts';

/** Local version numbers that a push up to `upTo` would send. */
export function versionsToPush(
  project: ProjectListItem,
  versions: VersionItem[] | undefined,
  upTo: number,
): number[] {
  const serverLatest = project.server_latest_version ?? 0;
  const local = (versions ?? [])
    .filter(
      (v) => !v.server_only && v.number > serverLatest && v.number <= upTo,
    )
    .map((v) => v.number)
    .sort((a, b) => a - b);
  return local.length > 0 ? local : [upTo];
}

/** Whether "Push to server" applies to this version. */
export function canPush(
  project: ProjectListItem,
  version: VersionItem,
  connected: boolean,
): boolean {
  if (!connected || version.server_only) return false;
  const linked = project.remotes.some((r) => r.kind === 'server');
  if (!linked) {
    // First push: offered on the newest local version.
    return (
      project.latest_version == null ||
      version.number === project.latest_version
    );
  }
  // Linked: only versions newer than the server's newest. When the server
  // could not be reached its latest version is unknown, so nothing shows.
  if (project.server_latest_version == null) return false;
  return version.number > project.server_latest_version;
}
