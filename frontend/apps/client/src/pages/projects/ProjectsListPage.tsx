// Owner: U1 (projects). The projects list (prototype ProjectsList,
// RemotesWrap, EmptyNotConnected, EmptyConnected, NewProjectMenu).
import { Button } from '@nebi/ui/components/ui/button';
import { Input } from '@nebi/ui/components/ui/input';
import { Spinner } from '@nebi/ui/components/ui/spinner';
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
import { clientApiError } from '@/api/client/base';
import { UninstallDialog } from '@/components/detail/UninstallDialog';
import { DeleteProjectDialog } from '@/components/projects/DeleteProjectDialog';
import { NewProjectButton } from '@/components/projects/NewProjectButton';
import { ProjectRow } from '@/components/projects/ProjectRow';
import { ProjectsEmptyState } from '@/components/projects/ProjectsEmptyState';
import { nextSort, searchAndSort } from '@/components/projects/projectList';
import { useProjects } from '@/hooks/client/projects';
import { useServerConnection } from '@/hooks/client/remote';
import type { ProjectSort, ProjectSortKey } from '@/types/client/projects';
import type { ProjectListItem } from '@/types/contracts';

const Heading = () => (
  <div className="flex flex-col gap-2">
    <h1 className="font-bold text-3xl text-foreground">Projects</h1>
    <p className="text-base text-muted-foreground">
      Local projects on this machine
    </p>
  </div>
);

export const ProjectsListPage = () => {
  const projects = useProjects();
  const connection = useServerConnection();
  const [query, setQuery] = useState('');
  const [sort, setSort] = useState<ProjectSort>({ key: 'name', dir: 'asc' });
  const [uninstallId, setUninstallId] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ProjectListItem | null>(
    null,
  );

  const all = projects.data ?? [];
  const rows = useMemo(
    () => searchAndSort(all, query, sort),
    [all, query, sort],
  );
  const uninstallTarget = all.find((p) => p.id === uninstallId);

  const header = (
    key: ProjectSortKey,
    label: ReactNode,
    className?: string,
  ) => {
    const active = sort.key === key;
    const Icon = active
      ? sort.dir === 'asc'
        ? ArrowUp
        : ArrowDown
      : ArrowUpDown;
    return (
      <TableHead
        className={className}
        onClick={() => setSort((s) => nextSort(s, key))}
        aria-sort={
          active ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none'
        }
      >
        <span className="flex-1">{label}</span>
        <Icon
          className={
            active
              ? 'size-3.5 text-foreground'
              : 'size-3.5 text-muted-foreground'
          }
          aria-hidden
        />
      </TableHead>
    );
  };

  let body: ReactNode;
  if (projects.isPending) {
    body = (
      <div className="mt-20 flex justify-center text-muted-foreground">
        <Spinner size="lg" label="Loading projects" />
      </div>
    );
  } else if (projects.isError) {
    body = (
      <section
        className="mt-20 flex flex-col items-center gap-3 text-center"
        aria-labelledby="error-title"
      >
        <h2
          id="error-title"
          className="font-semibold text-foreground text-xl leading-7"
        >
          Couldn’t load projects
        </h2>
        <p className="max-w-xl text-muted-foreground-strong text-sm">
          {clientApiError(projects.error)?.error ??
            'Nebi didn’t answer. Try again in a moment.'}
        </p>
        <Button variant="secondary" onClick={() => projects.refetch()}>
          Try again
        </Button>
      </section>
    );
  } else if (all.length === 0) {
    body = (
      <ProjectsEmptyState
        serverConnected={connection.data?.connected === true}
      />
    );
  } else {
    body = (
      <>
        <div className="relative w-80 max-w-full">
          <Search
            className="pointer-events-none absolute top-1/2 left-3 z-10 size-[18px] -translate-y-1/2 text-muted-foreground"
            aria-hidden
          />
          <Input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search projects"
            aria-label="Search projects"
            className="pl-9"
          />
        </div>
        <Table aria-label="Projects">
          <TableHeader>
            <TableRow>
              {header('name', 'Project name')}
              {header('version', 'Version', 'w-[200px]')}
              {header('environment', 'Environment', 'w-[196px]')}
              {header('size', 'Size', 'w-[194px]')}
              {header('remotes', 'Connected remotes', 'w-[280px]')}
              <TableHead className="w-[74px]">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((p) => (
              <ProjectRow
                key={p.id}
                project={p}
                onUninstall={(project) => setUninstallId(project.id)}
                onDelete={setDeleteTarget}
              />
            ))}
            {rows.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className="h-24 text-center text-muted-foreground"
                >
                  No projects match “{query}”.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </>
    );
  }

  // One <main> for every state so the heading stays mounted while the
  // list loads.
  return (
    <main className="flex flex-col gap-5 px-12 py-12 max-[640px]:px-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <Heading />
        {all.length > 0 && <NewProjectButton />}
      </div>
      {body}
      {uninstallTarget && (
        <UninstallDialog
          project={uninstallTarget}
          open
          onOpenChange={(open) => !open && setUninstallId(null)}
        />
      )}
      {deleteTarget && (
        <DeleteProjectDialog
          project={deleteTarget}
          open
          onOpenChange={(open) => !open && setDeleteTarget(null)}
        />
      )}
    </main>
  );
};
