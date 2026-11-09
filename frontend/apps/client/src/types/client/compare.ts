// Owner: the compare unit. Unit-specific frontend types; the API contract types
// live in types/contracts.ts (frozen).
import type { PackageChangeKind, PackageSource } from '@/types/contracts';

/** One side of a Compare versions row, numbered per side like a diff gutter. */
export interface CompareCell {
  line: number;
  name: string;
  version: string;
  /** Requested constraint, shown as a tooltip when it differs from version. */
  constraint?: string;
  source: PackageSource;
}

/** One Compare versions table row; a side is null when the package is absent. */
export interface CompareRow {
  key: string;
  name: string;
  change: PackageChangeKind;
  base: CompareCell | null;
  compare: CompareCell | null;
}

/** What the version pickers label: installed and the server's newest version. */
export interface CompareVersionMarks {
  installed: number | null;
  serverLatest: number | null;
  /** Versions that only exist on the linked server. */
  serverOnly: ReadonlySet<number>;
}
