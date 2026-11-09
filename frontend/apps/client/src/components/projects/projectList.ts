// Owner: U1 (projects). Pure helpers of the projects list: search, sort
// and the version a row shows or installs.

import type { ProjectSort, ProjectSortKey } from '@/types/client/projects';
import type { ProjectListItem } from '@/types/contracts';

/** The version "Install" restores: last installed, else the latest. */
export const installTarget = (p: ProjectListItem) =>
  p.last_installed_version ?? p.latest_version;

const sortValue: Record<
  ProjectSortKey,
  (p: ProjectListItem) => string | number
> = {
  name: (p) => p.name.toLowerCase(),
  version: (p) => p.installed_version ?? installTarget(p) ?? 0,
  environment: (p) => (p.installed_version != null ? 0 : 1),
  size: (p) => p.size_bytes ?? -1,
  remotes: (p) =>
    p.remotes
      .map((r) => r.name)
      .join(',')
      .toLowerCase(),
};

/** Filters by name (case-insensitive) and sorts, like the prototype. */
export function searchAndSort(
  projects: ProjectListItem[],
  query: string,
  sort: ProjectSort,
): ProjectListItem[] {
  const q = query.trim().toLowerCase();
  const value = sortValue[sort.key];
  return projects
    .filter((p) => p.name.toLowerCase().includes(q))
    .sort((a, b) => {
      const [x, y] = [value(a), value(b)];
      const cmp = x < y ? -1 : x > y ? 1 : 0;
      return sort.dir === 'asc' ? cmp : -cmp;
    });
}

/** Clicking the active column flips direction; another column sorts asc. */
export const nextSort = (
  current: ProjectSort,
  key: ProjectSortKey,
): ProjectSort =>
  current.key === key
    ? { key, dir: current.dir === 'asc' ? 'desc' : 'asc' }
    : { key, dir: 'asc' };
