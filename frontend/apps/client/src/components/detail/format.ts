// Owner: U2 (detail). Formatting helpers for the project detail page,
// matching the prototype's wording.
import type {
  ProjectListItem,
  SyncMarker,
  VersionItem,
} from '@/types/contracts';

const MONTHS = [
  'Jan',
  'Feb',
  'Mar',
  'Apr',
  'May',
  'Jun',
  'Jul',
  'Aug',
  'Sep',
  'Oct',
  'Nov',
  'Dec',
];

const DAY_MS = 24 * 60 * 60 * 1000;

/** Whole days between `iso` and `now` (0 = less than a day ago). */
export function ageInDays(iso: string, now = Date.now()): number {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return 0;
  return Math.max(0, Math.floor((now - t) / DAY_MS));
}

/** "9 Nov 2026"; "just now" within the last minute. */
export function formatVersionDate(iso: string, now = Date.now()): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return '';
  if (now - t >= 0 && now - t < 60_000) return 'just now';
  const d = new Date(t);
  return `${d.getDate()} ${MONTHS[d.getMonth()]} ${d.getFullYear()}`;
}

/** Rail group label: Today, This week, This month, This year, Older. */
export function ageGroupLabel(days: number): string {
  if (days < 1) return 'Today';
  if (days < 7) return 'This week';
  if (days < 31) return 'This month';
  if (days < 365) return 'This year';
  return 'Older';
}

export interface VersionGroup {
  label: string;
  versions: VersionItem[];
}

/** Groups versions (already newest first) by age, keeping their order. */
export function groupVersionsByAge(
  versions: VersionItem[],
  now = Date.now(),
): VersionGroup[] {
  const groups: VersionGroup[] = [];
  for (const v of versions) {
    const label = ageGroupLabel(ageInDays(v.created_at, now));
    const last = groups.at(-1);
    if (last?.label === label) last.versions.push(v);
    else groups.push({ label, versions: [v] });
  }
  return groups;
}

/** "version 5" / "versions 5, 6 and 7". */
export function versionsPhrase(numbers: number[]): string {
  if (numbers.length === 1) return `version ${numbers[0]}`;
  return `versions ${numbers.slice(0, -1).join(', ')} and ${numbers.at(-1)}`;
}

/** Capitalizes the first letter ("Versions 6 and 7"). */
export function capitalizeFirst(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** Download sizes in decimal units like the prototype ("12.3 MB"). */
export function formatBytes(bytes: number | null | undefined): string {
  if (!bytes || bytes <= 0) return '';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let i = 0;
  while (value >= 1000 && i < units.length - 1) {
    value /= 1000;
    i++;
  }
  const digits = i === 0 || value >= 100 ? 0 : 1;
  return `${value.toFixed(digits)} ${units[i]}`;
}

/** Maps the host's GOOS/GOARCH ("darwin/arm64") to a pixi platform. */
export function pixiPlatformFor(goPlatform: string | undefined): string {
  if (!goPlatform) return '';
  const [os, arch] = goPlatform.split('/');
  const archs: Record<string, Record<string, string>> = {
    linux: {
      amd64: 'linux-64',
      arm64: 'linux-aarch64',
      '386': 'linux-32',
      ppc64le: 'linux-ppc64le',
      s390x: 'linux-s390x',
      riscv64: 'linux-riscv64',
    },
    darwin: { amd64: 'osx-64', arm64: 'osx-arm64' },
    windows: { amd64: 'win-64', arm64: 'win-arm64', '386': 'win-32' },
    freebsd: { amd64: 'freebsd-64' },
  };
  return archs[os ?? '']?.[arch ?? ''] ?? '';
}

/** "linux-64 · this machine" for the host platform, else the bare name. */
export function platformLabel(platform: string, machine: string): string {
  return platform === machine ? `${platform} · this machine` : platform;
}

/** registry/repository:tag references of a version's publications. */
export function publishedReferences(v: VersionItem): string[] {
  return v.published.map(
    (p) => p.reference || `${p.registry}/${p.repository}:${p.tag}`,
  );
}

/** The local username the SDK reports for versions made on this machine. */
export const LOCAL_USERNAME = 'local-user';

/** Author display name with " (you)" for versions made on this machine. */
export function authorLabel(v: VersionItem): string {
  return v.author.username === LOCAL_USERNAME
    ? `${v.author.name} (you)`
    : v.author.name;
}

/** The project's server remote, if it is linked to one. */
export function serverRemote(project: ProjectListItem) {
  return project.remotes.find((r) => r.kind === 'server') ?? null;
}

/** Local versions the linked server does not have yet, oldest first. */
export function unpushedVersions(
  project: ProjectListItem,
  versions: VersionItem[],
): number[] {
  const serverLatest = project.server_latest_version ?? 0;
  return versions
    .filter((v) => !v.server_only && v.number > serverLatest)
    .map((v) => v.number)
    .sort((a, b) => a - b);
}

export const SYNC_MARKER_TIPS: Record<SyncMarker, string> = {
  newest_here: 'Latest on this machine. Not on the server yet.',
  newest_on_server: 'Latest on the server. Not on this machine yet.',
  in_sync: 'This machine and the server both have this version.',
};

/** The version to show: ?v=N, else the installed one, else the newest local. */
export function pickVersion(
  versions: VersionItem[],
  requested: number,
  installed: number | null,
): VersionItem | undefined {
  return (
    versions.find((v) => v.number === requested) ??
    versions.find((v) => v.number === installed && !v.server_only) ??
    versions.find((v) => !v.server_only) ??
    versions[0]
  );
}

/**
 * The reason an install failed, for the alert and toast under the title
 * "Version N couldn't install": drops the job error's own "version N
 * couldn't install:" prefix and ends it as a sentence.
 */
export function installErrorText(
  error: string | null | undefined,
  version: number,
): string {
  const prefix = new RegExp(
    `^version ${version} couldn[’']t install:\\s*`,
    'i',
  );
  const text = (error ?? '').trim().replace(prefix, '').trim();
  if (!text) return 'The install job failed. View the job for its log.';
  const sentence = text.charAt(0).toUpperCase() + text.slice(1);
  return /[.!?]$/.test(sentence) ? sentence : `${sentence}.`;
}
