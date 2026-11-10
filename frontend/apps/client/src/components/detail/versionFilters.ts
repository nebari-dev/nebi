// Owner: U2 (detail). Search and filters of the version rail.
import type { VersionItem } from '@/types/contracts';

export type VersionFilter =
  | 'installed'
  | 'tagged'
  | 'published'
  | 'not_pushed'
  | 'server_only';

export const VERSION_FILTER_LABELS: Record<VersionFilter, string> = {
  installed: 'Installed',
  tagged: 'Tagged',
  published: 'Published',
  not_pushed: 'Not on the server yet',
  server_only: 'Only on the server',
};

export interface VersionFilterContext {
  /** The linked server's newest version (null: not linked or empty). */
  serverLatest: number | null;
  /** The project is linked to a server. */
  linked: boolean;
}

/** Filters that make sense for this project (server ones need a link). */
export function availableFilters(
  versions: VersionItem[],
  ctx: VersionFilterContext,
): VersionFilter[] {
  const out: VersionFilter[] = ['installed', 'tagged', 'published'];
  if (ctx.linked) out.push('not_pushed');
  if (versions.some((v) => v.server_only)) out.push('server_only');
  return out;
}

function matchesFilter(
  v: VersionItem,
  f: VersionFilter,
  ctx: VersionFilterContext,
): boolean {
  switch (f) {
    case 'installed':
      return v.installed;
    case 'tagged':
      return v.tags.length > 0;
    case 'published':
      return v.published.length > 0;
    case 'not_pushed':
      return ctx.linked && !v.server_only && v.number > (ctx.serverLatest ?? 0);
    case 'server_only':
      return v.server_only;
  }
}

/**
 * Search (prototype rule: "v5" or "5" matches the number exactly; text
 * matches description, author or a tag) and filters (all must match).
 */
export function filterVersions(
  versions: VersionItem[],
  query: string,
  filters: VersionFilter[],
  ctx: VersionFilterContext,
): VersionItem[] {
  const q = query
    .trim()
    .toLowerCase()
    .replace(/^v(?=\d)/, '');
  return versions.filter((v) => {
    if (
      q &&
      String(v.number) !== q &&
      !v.description.toLowerCase().includes(q) &&
      !v.author.name.toLowerCase().includes(q) &&
      !v.tags.some((t) => t.toLowerCase().includes(q))
    ) {
      return false;
    }
    return filters.every((f) => matchesFilter(v, f, ctx));
  });
}
