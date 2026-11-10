// Server projects (prototype ServerUi): the team server's own UI lists the
// projects the signed-in user can see. Projects can be shared and deleted
// here, but not pulled or installed (that happens in the client app).
import { ConfirmDialog } from '@nebi/ui/components/confirm-dialog';
import { Badge } from '@nebi/ui/components/ui/badge';
import { Button } from '@nebi/ui/components/ui/button';
import {
  ButtonGroup,
  ButtonGroupSeparator,
} from '@nebi/ui/components/ui/button-group';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
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
import { toast } from '@nebi/ui/components/ui/toast';
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  Ellipsis,
  Search,
  Trash2,
  Users,
} from 'lucide-react';
import { useMemo, useState } from 'react';
import { ShareDialog } from '@/components/share/ShareDialog';
import { errorMessage } from '@/components/share/shareModel';
import { useIsAdmin } from '@/hooks/useAdmin';
import { useCatalogProjects, useDeleteServerProject } from '@/hooks/useSharing';
import type { CatalogProject } from '@/types/sharing';

type SortKey = 'name' | 'version' | 'namespace';
type Sort = { key: SortKey; dir: 'asc' | 'desc' };

const sortValue: Record<SortKey, (p: CatalogProject) => string | number> = {
  name: (p) => p.name.toLowerCase(),
  version: (p) => p.latest_version ?? -1,
  namespace: (p) => p.namespace.toLowerCase(),
};

export const ServerProjects = () => {
  const projects = useCatalogProjects();
  const { data: isAdmin } = useIsAdmin();
  const remove = useDeleteServerProject();
  const [search, setSearch] = useState('');
  const [sort, setSort] = useState<Sort>({ key: 'name', dir: 'asc' });
  const [sharing, setSharing] = useState<CatalogProject | null>(null);
  const [deleting, setDeleting] = useState<CatalogProject | null>(null);

  const rows = useMemo(() => {
    const q = search.trim().toLowerCase();
    const value = sortValue[sort.key];
    return (projects.data ?? [])
      .filter((p) => p.name.toLowerCase().includes(q))
      .sort((a, b) => {
        const [x, y] = [value(a), value(b)];
        const c = x < y ? -1 : x > y ? 1 : 0;
        return sort.dir === 'asc' ? c : -c;
      });
  }, [projects.data, search, sort]);

  const sortHead = (key: SortKey, label: string, className?: string) => {
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

  const canDelete = (p: CatalogProject) => p.role === 'owner' || !!isAdmin;

  const confirmDelete = () => {
    const project = deleting;
    if (!project) return;
    remove.mutate(project.id, {
      onSuccess: () => {
        setDeleting(null);
        toast.success(`Deleted ${project.name} from the server`, {
          description: 'Copies already pulled to a machine keep working.',
        });
      },
      onError: (err) => {
        setDeleting(null);
        toast.error(`Couldn’t delete ${project.name}`, {
          description: errorMessage(err, 'The server did not delete it.'),
        });
      },
    });
  };

  return (
    <main className="flex flex-col gap-5 px-12 py-12">
      <div className="flex flex-col gap-2">
        <h1 className="font-bold text-3xl text-foreground">Server projects</h1>
        <p className="text-base text-muted-foreground">
          This is a server-only interface. Projects can be managed and shared
          here, but not pulled or installed.
        </p>
      </div>

      <div className="relative w-80">
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
            {sortHead('name', 'Project')}
            {sortHead('version', 'Latest version', 'w-[150px]')}
            {sortHead('namespace', 'Namespace', 'w-[300px]')}
            <TableHead className="w-[112px]">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {projects.isPending ? (
            <TableRow>
              <TableCell colSpan={4} className="h-24 text-center">
                <span className="inline-flex items-center gap-2 text-muted-foreground">
                  <Spinner size="sm" />
                  Loading server projects…
                </span>
              </TableCell>
            </TableRow>
          ) : projects.isError ? (
            <TableRow>
              <TableCell colSpan={4} className="h-24 text-center">
                <span className="inline-flex items-center gap-3">
                  <span className="text-destructive-foreground">
                    {errorMessage(
                      projects.error,
                      'Couldn’t load server projects.',
                    )}
                  </span>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => projects.refetch()}
                  >
                    Retry
                  </Button>
                </span>
              </TableCell>
            </TableRow>
          ) : rows.length === 0 ? (
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
          ) : (
            rows.map((p) => (
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
                <TableCell className="h-10 py-1">
                  <ButtonGroup aria-label={`Actions for ${p.name}`}>
                    <Button variant="outline" onClick={() => setSharing(p)}>
                      Share
                    </Button>
                    <ButtonGroupSeparator />
                    <DropdownMenu modal={false}>
                      <DropdownMenuTrigger
                        variant="outline"
                        aria-label={`More actions for ${p.name}`}
                        className="w-8 px-0"
                      >
                        <Ellipsis />
                      </DropdownMenuTrigger>
                      <DropdownMenuPortal>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => setSharing(p)}>
                            <Users aria-hidden />
                            Manage access
                          </DropdownMenuItem>
                          {canDelete(p) && (
                            <DropdownMenuItem
                              variant="destructive"
                              onClick={() => setDeleting(p)}
                            >
                              <Trash2 aria-hidden />
                              Delete from server…
                            </DropdownMenuItem>
                          )}
                        </DropdownMenuContent>
                      </DropdownMenuPortal>
                    </DropdownMenu>
                  </ButtonGroup>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>

      {sharing && (
        <ShareDialog
          projectId={sharing.id}
          projectName={sharing.name}
          open
          onOpenChange={(open) => !open && setSharing(null)}
        />
      )}

      <ConfirmDialog
        open={deleting != null}
        onOpenChange={(open) => {
          if (!open && !remove.isPending) setDeleting(null);
        }}
        onConfirm={confirmDelete}
        title={`Delete ${deleting?.name ?? ''} from the server?`}
        description="Everyone loses access to it on the server and its versions are removed there. Copies already pulled to a machine keep working."
        confirmText="Delete from server"
        variant="destructive"
        confirmLoading={remove.isPending}
        keepOpenOnConfirm
      />
    </main>
  );
};
