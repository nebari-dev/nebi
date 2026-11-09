// Owner: the projects unit. Unit-specific frontend types; the API contract types
// live in types/contracts.ts (frozen).

/** Sortable columns of the projects list. */
export type ProjectSortKey =
  | 'name'
  | 'version'
  | 'environment'
  | 'size'
  | 'remotes';

export interface ProjectSort {
  key: ProjectSortKey;
  dir: 'asc' | 'desc';
}
