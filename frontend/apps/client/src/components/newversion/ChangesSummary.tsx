// Owner: U3 (newversion). "Summary of changes": what the draft adds,
// removes and changes compared with its base version (requested packages,
// channels, platforms), in the colors of Compare versions.
import { cn } from '@nebi/ui/lib/utils';
import { Loader2 } from 'lucide-react';
import type { PackageDiffEntry, VersionDiff } from '@/types/contracts';
import { hasChanges } from './changes';

const CHANGE_CLASS = {
  added: 'text-success-foreground',
  removed: 'text-destructive-foreground',
  changed: 'text-warning-foreground',
  same: 'text-muted-foreground',
} as const;

const SIGN = { added: '+', removed: '−', changed: '~', same: '' } as const;

const constraint = (e: PackageDiffEntry, side: 'base' | 'compare') =>
  e[side]?.constraint ?? e[side]?.version ?? '';

const display = (c: string) => (c === '*' || c === '' ? 'any version' : c);

function PackageRow({ entry }: { entry: PackageDiffEntry }) {
  const label =
    entry.change === 'added'
      ? display(constraint(entry, 'compare'))
      : entry.change === 'removed'
        ? display(constraint(entry, 'base'))
        : `${display(constraint(entry, 'base'))} → ${display(constraint(entry, 'compare'))}`;
  return (
    <li className="flex items-center gap-3 text-sm">
      <span
        className={cn(
          'w-4 shrink-0 text-center font-mono',
          CHANGE_CLASS[entry.change],
        )}
      >
        <span aria-hidden>{SIGN[entry.change]}</span>
        <span className="sr-only">{entry.change}</span>
      </span>
      <span className="min-w-0 truncate font-mono text-foreground">
        {entry.name}
        {entry.source === 'pypi' && (
          <span className="text-muted-foreground"> (PyPI)</span>
        )}
      </span>
      <span
        className={cn(
          'ml-auto shrink-0 font-mono text-xs',
          CHANGE_CLASS[entry.change],
        )}
      >
        {label}
      </span>
    </li>
  );
}

function ListChange({
  label,
  added,
  removed,
}: {
  label: string;
  added: string[] | null;
  removed: string[] | null;
}) {
  if (!added?.length && !removed?.length) return null;
  return (
    <p className="text-sm">
      <span className="text-muted-foreground">{label}: </span>
      {added?.length ? (
        <span className={CHANGE_CLASS.added}>added {added.join(', ')}</span>
      ) : null}
      {added?.length && removed?.length ? (
        <span className="text-muted-foreground"> · </span>
      ) : null}
      {removed?.length ? (
        <span className={CHANGE_CLASS.removed}>
          removed {removed.join(', ')}
        </span>
      ) : null}
    </p>
  );
}

export interface ChangesSummaryProps {
  projectName: string;
  diff: VersionDiff | undefined;
  loading?: boolean;
  /** Message to show instead of the summary (e.g. pixi.toml can't be read). */
  error?: string | null;
}

export function ChangesSummary({
  projectName,
  diff,
  loading,
  error,
}: ChangesSummaryProps) {
  if (error) {
    return <p className="text-muted-foreground text-sm">{error}</p>;
  }
  if (!diff) {
    return (
      <p className="flex items-center gap-2 text-muted-foreground text-sm">
        {loading && <Loader2 className="size-4 animate-spin" aria-hidden />}
        Comparing…
      </p>
    );
  }
  if (!hasChanges(diff)) {
    return (
      <p className="text-muted-foreground text-sm" aria-live="polite">
        Nothing changed in {projectName}.
      </p>
    );
  }
  const rows = (diff.requested ?? []).filter((e) => e.change !== 'same');
  const c = diff.requested_counts;
  return (
    <div
      className="flex flex-col gap-3 [font-variant-ligatures:none]"
      aria-live="polite"
    >
      <p
        className="flex items-center gap-3 font-medium text-sm leading-5"
        role="img"
        aria-label={`Summary of changes: ${c.added} added, ${c.removed} removed, ${c.changed} changed`}
      >
        <span className={CHANGE_CLASS.added} title="Added">
          +{c.added}
        </span>
        <span className={CHANGE_CLASS.removed} title="Removed">
          −{c.removed}
        </span>
        <span className={CHANGE_CLASS.changed} title="Changed">
          ~{c.changed}
        </span>
        {loading && (
          <Loader2
            className="size-4 animate-spin text-muted-foreground"
            aria-hidden
          />
        )}
      </p>
      {rows.length > 0 && (
        <ul
          className="flex flex-col gap-1.5 rounded-md border border-border bg-card p-3"
          aria-label="Package changes"
        >
          {rows.map((e) => (
            <PackageRow key={`${e.source}:${e.name}`} entry={e} />
          ))}
        </ul>
      )}
      <ListChange
        label="Channels"
        added={diff.channels.added}
        removed={diff.channels.removed}
      />
      <ListChange
        label="Platforms"
        added={diff.platforms.added}
        removed={diff.platforms.removed}
      />
    </div>
  );
}
