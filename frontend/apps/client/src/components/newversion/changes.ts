// Owner: U3 (newversion). Pure helpers for the new version page: change
// summaries in the prototype's wording, tag validation, paths.
import type {
  ManifestForm,
  PackageDiffEntry,
  VersionDiff,
} from '@/types/contracts';

/** Path of the new version page starting from version `from`. */
export const newVersionPath = (projectId: string, from: number) =>
  `/projects/${projectId}/new-version?from=${from}`;

/** "version 6" / "versions 5 and 6" / "versions 4, 5 and 6". */
export function versionList(numbers: number[]): string {
  if (numbers.length === 1) return `version ${numbers[0]}`;
  return `versions ${numbers.slice(0, -1).join(', ')} and ${numbers.at(-1)}`;
}

const joinAnd = (xs: string[]) =>
  xs.length <= 2
    ? xs.join(' and ')
    : `${xs.slice(0, -1).join(', ')} and ${xs.at(-1)}`;

const constraintOf = (e: PackageDiffEntry, side: 'base' | 'compare') =>
  e[side]?.constraint ?? e[side]?.version ?? '';

/** A requested package whose constraint differs between the two sides. */
const constraintChanged = (e: PackageDiffEntry) =>
  e.change === 'changed' &&
  constraintOf(e, 'base') !== constraintOf(e, 'compare');

/**
 * Requested-package changes in the prototype's wording:
 * ["added pandas and rich", "removed numpy", "changed python"].
 */
export function summarizeRequested(diff: VersionDiff | undefined): string[] {
  if (!diff) return [];
  const rows = diff.requested ?? [];
  const added = rows.filter((e) => e.change === 'added').map((e) => e.name);
  const removed = rows.filter((e) => e.change === 'removed').map((e) => e.name);
  const changed = rows.filter(constraintChanged).map((e) => e.name);
  return [
    added.length ? `added ${joinAnd(added)}` : null,
    removed.length ? `removed ${joinAnd(removed)}` : null,
    changed.length ? `changed ${joinAnd(changed)}` : null,
  ].filter((s): s is string => s != null);
}

/** "Added pandas and rich, removed numpy" ("Suggested from your changes."). */
export function suggestDescription(diff: VersionDiff | undefined): string {
  const parts = summarizeRequested(diff);
  if (!parts.length) return '';
  const s = parts.join(', ');
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** True when the draft differs from its base in packages, channels or platforms. */
export function hasChanges(diff: VersionDiff | undefined): boolean {
  if (!diff) return false;
  const c = diff.requested_counts;
  return (
    c.added + c.removed + c.changed > 0 ||
    (diff.channels.added?.length ?? 0) > 0 ||
    (diff.channels.removed?.length ?? 0) > 0 ||
    (diff.platforms.added?.length ?? 0) > 0 ||
    (diff.platforms.removed?.length ?? 0) > 0
  );
}

const TAG_PATTERN = /^[A-Za-z][A-Za-z0-9._-]*$/;

/** Prototype tag rule; null when the tag can be used. */
export function tagError(tag: string): string | null {
  if (TAG_PATTERN.test(tag)) return null;
  return /^[A-Za-z]/.test(tag)
    ? `“${tag}” can’t be used: tags use letters, numbers, dots, hyphens and underscores.`
    : `“${tag}” can’t be used: tags must start with a letter.`;
}

/** Tags sent to the API: "latest" moves on its own and is never sent. */
export const tagsForRequest = (tags: string[]) =>
  tags.filter((t) => t !== 'latest');

/** Deep equality of two manifest forms (order matters, like the TOML). */
export function sameForm(a: ManifestForm, b: ManifestForm): boolean {
  const norm = (f: ManifestForm) =>
    JSON.stringify({
      name: f.name,
      description: f.description ?? '',
      channels: f.channels,
      platforms: f.platforms,
      requested: f.requested.map((p) => ({
        name: p.name,
        constraint: p.constraint,
        source: p.source && p.source !== 'conda' ? p.source : 'conda',
      })),
    });
  return norm(a) === norm(b);
}
