// Owner: U4 (compare). Diff colours and markers from the prototype.
import type { PackageChangeKind } from '@/types/contracts';

export const CHANGE_MARKER: Record<PackageChangeKind, string> = {
  added: '+',
  removed: '−',
  changed: '~',
  same: '',
};

export const CHANGE_BG: Record<PackageChangeKind, string> = {
  added: 'bg-success',
  removed: 'bg-destructive',
  changed: 'bg-warning',
  same: 'bg-card',
};

export const CHANGE_TEXT: Record<PackageChangeKind, string> = {
  added: 'text-success-foreground',
  removed: 'text-destructive-foreground',
  changed: 'text-warning-foreground',
  same: 'text-muted-foreground',
};
