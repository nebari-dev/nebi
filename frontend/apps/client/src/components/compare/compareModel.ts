// Owner: U4 (compare). Pure helpers for the Compare versions page.

import type {
  CompareCell,
  CompareRow,
  CompareVersionMarks,
} from '@/types/client/compare';
import type {
  CompareResult,
  ListDiff,
  PackageDiffEntry,
  PackageRef,
  ProjectListItem,
  VersionItem,
} from '@/types/contracts';

/**
 * Installed and server-latest versions for the picker labels. The project
 * row is authoritative; the version rail's markers fill in when the project
 * has not been decorated with the server's latest version.
 */
export function versionMarks(
  project: ProjectListItem | undefined,
  versions: VersionItem[],
): CompareVersionMarks {
  const installed =
    project?.installed_version ??
    versions.find((v) => v.installed)?.number ??
    null;
  const serverLatest =
    project?.server_latest_version ??
    versions.find((v) => v.sync === 'newest_on_server' || v.sync === 'in_sync')
      ?.number ??
    null;
  return {
    installed,
    serverLatest,
    serverOnly: new Set(
      versions.filter((v) => v.server_only).map((v) => v.number),
    ),
  };
}

/** "v7 (installed, latest on server)", as the prototype labels versions. */
export function formatVersionLabel(n: number, marks: CompareVersionMarks) {
  const notes: string[] = [];
  if (marks.installed === n) notes.push('installed');
  if (marks.serverLatest === n) notes.push('latest on server');
  else if (marks.serverOnly.has(n)) notes.push('on server');
  return notes.length ? `v${n} (${notes.join(', ')})` : `v${n}`;
}

/** Version numbers newest first, without duplicates. */
export function versionNumbers(versions: VersionItem[]): number[] {
  return [...new Set(versions.map((v) => v.number))].sort((a, b) => b - a);
}

const parseVersion = (raw: string | null): number | null => {
  if (raw == null || raw === '') return null;
  const n = Number(raw);
  return Number.isInteger(n) && n > 0 ? n : null;
};

/**
 * Default Base and Compare to (prototype rules). Base: the version the page
 * was opened from, else the installed version, else the newest version the
 * server also has, else the newest. Compare to: the installed version, else
 * the server's latest, whichever differs from Base, else the newest other
 * version.
 */
export function defaultSelection(
  numbers: number[],
  marks: CompareVersionMarks,
  from: number | null,
): { base: number; target: number } | null {
  if (numbers.length === 0) return null;
  const has = (n: number | null): n is number =>
    n != null && numbers.includes(n);
  const base =
    (has(from) ? from : null) ??
    (has(marks.installed) ? marks.installed : null) ??
    numbers.find(
      (n) => n <= (marks.serverLatest ?? Number.POSITIVE_INFINITY),
    ) ??
    numbers[0];
  const candidates = [
    has(marks.installed) ? marks.installed : null,
    has(marks.serverLatest) ? marks.serverLatest : null,
  ];
  const target =
    candidates.find((n): n is number => n != null && n !== base) ??
    numbers.find((n) => n !== base) ??
    base;
  return { base, target };
}

/**
 * Base and Compare to from the URL (?base=&to=), falling back to the
 * defaults for a missing or unknown version.
 */
export function resolveSelection(
  numbers: number[],
  marks: CompareVersionMarks,
  params: URLSearchParams,
): { base: number; target: number; from: number | null } | null {
  const from = parseVersion(params.get('from'));
  const defaults = defaultSelection(numbers, marks, from);
  if (!defaults) return null;
  const pick = (key: string, fallback: number) => {
    const n = parseVersion(params.get(key));
    return n != null && numbers.includes(n) ? n : fallback;
  };
  return {
    base: pick('base', defaults.base),
    target: pick('to', defaults.target),
    from,
  };
}

const cell = (
  ref: PackageRef | null,
  entry: PackageDiffEntry,
  line: number,
): CompareCell | null => {
  if (!ref) return null;
  const showConstraint =
    entry.change === 'changed' &&
    (entry.base?.constraint ?? '') !== (entry.compare?.constraint ?? '') &&
    !!ref.constraint &&
    ref.constraint !== ref.version;
  return {
    line,
    name: entry.name,
    version: ref.version,
    constraint: ref.constraint,
    source: entry.source,
    ...(showConstraint ? { showConstraint } : {}),
  };
};

/** Table rows with per-side line numbers (a side counts only its packages). */
export function buildCompareRows(entries: PackageDiffEntry[]): CompareRow[] {
  let baseLine = 0;
  let compareLine = 0;
  return entries.map((e) => ({
    key: `${e.source}:${e.name}`,
    name: e.name,
    change: e.change,
    base: cell(e.base, e, e.base ? ++baseLine : 0),
    compare: cell(e.compare, e, e.compare ? ++compareLine : 0),
  }));
}

/** "added bioconda; removed defaults", or "" when the list is unchanged. */
export function describeListDiff(diff: ListDiff): string {
  const parts: string[] = [];
  if (diff.added?.length) parts.push(`added ${diff.added.join(', ')}`);
  if (diff.removed?.length) parts.push(`removed ${diff.removed.join(', ')}`);
  return parts.join('; ');
}

/** Whether channels or platforms differ between the two versions. */
export const hasSpecChanges = (r: CompareResult) =>
  describeListDiff(r.channels) !== '' || describeListDiff(r.platforms) !== '';
