// Owner: U5 (remote). Server projects table with search and sortable
// columns (prototype ServerConnected): Project, Latest version, Namespace,
// Actions.
import { Badge } from '@nebi/ui/components/ui/badge';
import { Input } from '@nebi/ui/components/ui/input';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@nebi/ui/components/ui/table';
import { ArrowDown, ArrowUp, ArrowUpDown, Search } from 'lucide-react';
import { type ReactNode, useMemo, useState } from 'react';
import type { ServerProjectItem } from '@/types/contracts';

type SortKey = 'name' | 'version' | 'namespace';
type Sort = { key: SortKey; dir: 'asc' | 'desc' };

const sortValue: Record<SortKey, (p: ServerProjectItem) => string | number> = {
  name: (p) => p.name.toLowerCase(),
  version: (p) => p.latest_version ?? 0,
  namespace: (p) => p.namespace.toLowerCase(),
};

export interface ServerProjectsTableProps {
  projects: ServerProjectItem[];
  actions: (project: ServerProjectItem) => ReactNode;
  actionsWidth?: string;
}

export const ServerProjectsTable = ({
  projects,
  actions,
  actionsWidth = 'w-[180px]',
}: ServerProjectsTableProps) => {
  const [search, setSearch] = useState('');
  const [sort, setSort] = useState<Sort>({ key: 'name', dir: 'asc' });

  const rows = useMemo(() => {
    const q = search.trim().toLowerCase();
    const value = sortValue[sort.key];
    return projects
      .filter((p) => p.name.toLowerCase().includes(q))
      .sort((a, b) => {
        const [x, y] = [value(a), value(b)];
        const c = x < y ? -1 : x > y ? 1 : 0;
        return sort.dir === 'asc' ? c : -c;
      });
  }, [projects, search, sort]);

  const head = (key: SortKey, label: string, className?: string) => {
    const active = sort.key === key;
    const Icon = active
      ? sort.dir === 'asc'
        ? ArrowUp
        : ArrowDown
      : ArrowUpDown;
    return (
      <TableHead
        className={className}
        aria-sort={
          active ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none'
        }
        onClick={() =>
          setSort((s) =>
            s.key === key
              ? { key, dir: s.dir === 'asc' ? 'desc' : 'asc' }
              : { key, dir: 'asc' },
          )
        }
      >
        <span className="flex-1">{label}</span>
        <Icon
          aria-hidden
          className={
            active
              ? 'size-3.5 text-foreground'
              : 'size-3.5 text-muted-foreground'
          }
        />
      </TableHead>
    );
  };

  return (
    <>
      <div className="relative w-80 max-w-full">
        <Search
          aria-hidden
          className="pointer-events-none absolute top-1/2 left-3 z-10 size-[18px] -translate-y-1/2 text-muted-foreground"
        />
        <Input
          type="search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search server projects"
          aria-label="Search server projects"
          className="pl-9"
        />
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            {head('name', 'Project')}
            {head('version', 'Latest version', 'w-[150px]')}
            {head('namespace', 'Namespace', 'w-[300px]')}
            <TableHead className={actionsWidth}>Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((p) => (
            <TableRow key={p.id}>
              <TableCell className="h-10 py-2">{p.name}</TableCell>
              <TableCell className="h-10 py-2">
                {p.latest_version != null ? (
                  <Badge className="bg-muted text-muted-foreground-strong">
                    v{p.latest_version}
                  </Badge>
                ) : (
                  <span className="text-muted-foreground">No versions</span>
                )}
              </TableCell>
              <TableCell className="h-10 py-2">{p.namespace}</TableCell>
              <TableCell className="h-10 py-1">{actions(p)}</TableCell>
            </TableRow>
          ))}
          {rows.length === 0 && (
            <TableRow>
              <TableCell
                colSpan={4}
                className="h-24 text-center text-muted-foreground"
              >
                {search
                  ? `No server projects match “${search}”.`
                  : 'Nothing is shared with you on this server yet.'}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </>
  );
};
