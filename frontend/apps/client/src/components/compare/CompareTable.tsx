// Owner: U4 (compare). Side-by-side requested packages of two versions.
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@nebi/ui/components/ui/table';
import { cn } from '@nebi/ui/lib/utils';
import type { CompareCell, CompareRow } from '@/types/client/compare';
import type { PackageChangeKind } from '@/types/contracts';
import { CHANGE_BG, CHANGE_MARKER, CHANGE_TEXT } from './changeStyles';

const Side = ({
  cell,
  change,
}: {
  cell: CompareCell | null;
  change: PackageChangeKind;
}) => {
  const bg = cell ? CHANGE_BG[change] : 'bg-muted/40';
  return (
    <>
      <TableCell className="h-auto border-border border-r bg-muted px-2 py-2.5 text-right align-top font-mono text-muted-foreground text-xs leading-4">
        {cell?.line}
      </TableCell>
      <TableCell
        className={cn(
          'h-auto px-1 py-2.5 text-center align-top font-mono text-xs leading-4',
          bg,
          CHANGE_TEXT[change],
        )}
        aria-label={cell && change !== 'same' ? change : undefined}
      >
        {cell && CHANGE_MARKER[change]}
      </TableCell>
      <TableCell className={cn('h-auto py-2.5 pr-4 pl-2 font-mono', bg)}>
        {cell ? (
          <span className="flex items-center justify-between gap-4">
            <span className="flex min-w-0 items-baseline gap-2">
              <span className="truncate text-foreground text-sm leading-5">
                {cell.name}
              </span>
              {cell.source === 'pypi' && (
                <span className="shrink-0 text-muted-foreground text-xs leading-4">
                  PyPI
                </span>
              )}
            </span>
            <span
              className={cn('shrink-0 text-xs leading-4', CHANGE_TEXT[change])}
              title={
                cell.constraint && cell.constraint !== cell.version
                  ? `Requested ${cell.constraint}`
                  : undefined
              }
            >
              {cell.version}
            </span>
          </span>
        ) : (
          <span className="text-muted-foreground text-xs leading-4">
            <span aria-hidden="true">–</span>
            <span className="sr-only">Not in this version</span>
          </span>
        )}
      </TableCell>
    </>
  );
};

export const CompareTable = ({
  rows,
  baseLabel,
  targetLabel,
  busy,
}: {
  rows: CompareRow[];
  baseLabel: string;
  targetLabel: string;
  busy?: boolean;
}) => (
  <Table
    aria-label={`Requested packages, ${baseLabel} compared to ${targetLabel}`}
    aria-busy={busy || undefined}
    className={cn(
      'min-w-[40rem] table-fixed motion-safe:transition-opacity',
      busy && 'opacity-60',
    )}
  >
    <colgroup>
      <col className="w-9" />
      <col className="w-6" />
      <col />
      <col className="w-10" />
      <col className="w-9" />
      <col className="w-6" />
      <col />
    </colgroup>
    <TableHeader>
      <TableRow className="hover:bg-transparent">
        <TableHead colSpan={3} className="px-4 font-semibold text-base">
          {baseLabel}
        </TableHead>
        <TableHead aria-hidden="true" />
        <TableHead colSpan={3} className="px-4 font-semibold text-base">
          {targetLabel}
        </TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      {rows.length === 0 ? (
        <TableRow className="hover:bg-transparent">
          <TableCell
            colSpan={7}
            className="py-6 text-center text-muted-foreground"
          >
            Neither version requests any packages.
          </TableCell>
        </TableRow>
      ) : (
        rows.map((row) => (
          <TableRow key={row.key} className="hover:bg-transparent">
            <Side cell={row.base} change={row.change} />
            <TableCell aria-hidden="true" className="h-auto bg-card p-0" />
            <Side cell={row.compare} change={row.change} />
          </TableRow>
        ))
      )}
    </TableBody>
  </Table>
);
