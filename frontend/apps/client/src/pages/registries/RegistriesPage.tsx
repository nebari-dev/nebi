// Owner: U7 (publish). Registries page (issue #649). The prototype stubs
// this page ("The Registries page isn't in the prototype yet"); it follows
// the Server page layout: title and subtitle, actions on the right, a
// search box and a table with a row menu.
import { ConfirmDialog } from '@nebi/ui/components/confirm-dialog';
import { Badge } from '@nebi/ui/components/ui/badge';
import { Button } from '@nebi/ui/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
import { Input } from '@nebi/ui/components/ui/input';
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
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@nebi/ui/components/ui/tooltip';
import {
  Download,
  Ellipsis,
  Pencil,
  Plus,
  Search,
  Star,
  Trash,
} from 'lucide-react';
import { type ReactNode, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { clientApiError } from '@/api/client/base';
import { AddRegistryDialog } from '@/components/registries/AddRegistryDialog';
import { PullFromRegistryDialog } from '@/components/registries/PullFromRegistryDialog';
import {
  useRegistries,
  useRemoveRegistry,
  useUpdateRegistry,
} from '@/hooks/client/publish';
import type { RegistryItem } from '@/types/contracts';

export const RegistriesPage = () => {
  const navigate = useNavigate();
  const registries = useRegistries();
  const remove = useRemoveRegistry();
  const update = useUpdateRegistry();
  const [search, setSearch] = useState('');
  const [addOpen, setAddOpen] = useState(false);
  const [editing, setEditing] = useState<RegistryItem | null>(null);
  const [removing, setRemoving] = useState<RegistryItem | null>(null);
  const [pullFrom, setPullFrom] = useState<string | null>(null);

  const list = registries.data ?? [];
  const shown = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return list;
    return list.filter((r) =>
      [r.name, r.url, r.namespace].some((v) => v.toLowerCase().includes(q)),
    );
  }, [list, search]);

  const makeDefault = async (r: RegistryItem) => {
    try {
      await update.mutateAsync({ id: r.id, is_default: true });
      toast.add({
        title: `${r.name} is the default registry`,
        type: 'success',
      });
    } catch (err) {
      toast.add({
        title: 'Couldn’t change the default registry',
        description: clientApiError(err)?.error,
        type: 'error',
      });
    }
  };

  const confirmRemove = async () => {
    if (!removing) return;
    const r = removing;
    try {
      await remove.mutateAsync(r.id);
      toast.add({
        title: `${r.name} removed`,
        description: 'Tags you published there stay in the registry.',
        type: 'success',
      });
      setRemoving(null);
    } catch (err) {
      toast.add({
        title: `Couldn’t remove ${r.name}`,
        description: clientApiError(err)?.error,
        type: 'error',
      });
    }
  };

  const header = (
    <div className="flex flex-col gap-2">
      <h1 className="font-bold text-3xl text-foreground">Registries</h1>
      <p className="text-base text-muted-foreground">
        OCI registries this machine publishes to and pulls from
      </p>
    </div>
  );

  const dialogs = (
    <>
      <AddRegistryDialog
        open={addOpen || editing !== null}
        registry={editing}
        onOpenChange={(open) => {
          if (!open) {
            setAddOpen(false);
            setEditing(null);
          }
        }}
      />
      <PullFromRegistryDialog
        open={pullFrom !== null}
        registryId={pullFrom || undefined}
        onOpenChange={(open) => !open && setPullFrom(null)}
        onPulled={(id) => navigate(`/projects/${id}`)}
      />
      <ConfirmDialog
        open={removing !== null}
        onOpenChange={(open) => !open && setRemoving(null)}
        onConfirm={confirmRemove}
        keepOpenOnConfirm
        confirmLoading={remove.isPending}
        variant="destructive"
        confirmText="Remove"
        title={`Remove ${removing?.name ?? 'registry'}?`}
        description="Nebi forgets this registry and its saved password. Tags you published stay in the registry, but you can’t unpublish them from this machine anymore."
        className="max-w-[520px]"
      />
    </>
  );

  // One <main> and header for every state, so the h1 stays mounted while
  // the list loads.
  const shell = (actions: ReactNode, body: ReactNode) => (
    <main className="flex flex-col gap-5 px-12 py-12">
      <div className="flex items-center justify-between gap-4">
        {header}
        {actions}
      </div>
      {body}
      {dialogs}
    </main>
  );

  if (registries.isPending) {
    return shell(
      null,
      <p className="text-muted-foreground text-sm">Loading registries…</p>,
    );
  }

  if (registries.isError) {
    return shell(
      null,
      <>
        <p role="alert" className="text-destructive-foreground text-sm">
          {clientApiError(registries.error)?.error ??
            'Couldn’t load your registries.'}
        </p>
        <Button
          variant="secondary"
          className="w-fit"
          onClick={() => registries.refetch()}
        >
          Try again
        </Button>
      </>,
    );
  }

  if (list.length === 0) {
    return shell(
      null,
      <section
        className="mt-20 flex flex-col items-center gap-3 text-center"
        aria-labelledby="registries-empty-title"
      >
        <h2
          id="registries-empty-title"
          className="font-semibold text-foreground text-xl leading-7"
        >
          No registries yet
        </h2>
        <p className="max-w-xl text-muted-foreground-strong text-sm">
          Add a registry to publish versions to it and pull projects from it.
        </p>
        <div className="mt-2 flex flex-wrap justify-center gap-2">
          <Button onClick={() => setAddOpen(true)}>
            <Plus />
            Add a registry
          </Button>
        </div>
      </section>,
    );
  }

  return shell(
    <div className="flex items-center gap-2">
      <Button variant="secondary" onClick={() => setPullFrom('')}>
        <Download />
        Pull from registry
      </Button>
      <Button onClick={() => setAddOpen(true)}>
        <Plus />
        Add a registry
      </Button>
    </div>,
    <>
      <div className="relative w-80">
        <Search
          className="pointer-events-none absolute top-1/2 left-3 z-10 size-[18px] -translate-y-1/2 text-muted-foreground"
          aria-hidden
        />
        <Input
          type="search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search registries"
          aria-label="Search registries"
          className="pl-9"
        />
      </div>
      <Table aria-label="Registries">
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead className="w-[260px]">URL</TableHead>
            <TableHead className="w-[200px]">Namespace</TableHead>
            <TableHead className="w-[180px]">Credentials</TableHead>
            <TableHead className="w-[150px]">Unpublish</TableHead>
            <TableHead className="w-[74px]">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {shown.map((r) => (
            <TableRow key={r.id}>
              <TableCell className="h-10 py-2">
                <span className="flex items-center gap-2">
                  {r.name}
                  {r.is_default && <Badge variant="secondary">Default</Badge>}
                </span>
              </TableCell>
              <TableCell className="h-10 py-2 font-mono text-xs!">
                {r.url}
              </TableCell>
              <TableCell className="h-10 py-2">
                {r.namespace || (
                  <span className="text-muted-foreground">None</span>
                )}
              </TableCell>
              <TableCell className="h-10 py-2">
                {r.has_credentials ? (
                  <span>Signed in as {r.username}</span>
                ) : (
                  <span className="text-muted-foreground">None</span>
                )}
              </TableCell>
              <TableCell className="h-10 py-2">
                {r.can_unpublish ? (
                  'Supported'
                ) : (
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <button
                          type="button"
                          className="cursor-help rounded-sm text-muted-foreground underline decoration-dotted underline-offset-4 outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        />
                      }
                    >
                      Not supported
                    </TooltipTrigger>
                    <TooltipContent>{r.unpublish_reason}</TooltipContent>
                  </Tooltip>
                )}
              </TableCell>
              <TableCell className="h-10 py-1">
                <DropdownMenu>
                  <DropdownMenuTrigger
                    variant="ghost"
                    className="size-7 px-0"
                    aria-label={`Actions for ${r.name}`}
                  >
                    <Ellipsis />
                  </DropdownMenuTrigger>
                  <DropdownMenuPortal>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => setPullFrom(r.id)}>
                        <Download />
                        Pull from this registry
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => setEditing(r)}>
                        <Pencil />
                        Edit
                      </DropdownMenuItem>
                      {!r.is_default && (
                        <DropdownMenuItem onClick={() => makeDefault(r)}>
                          <Star />
                          Make default
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        onClick={() => setRemoving(r)}
                      >
                        <Trash />
                        Remove…
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenuPortal>
                </DropdownMenu>
              </TableCell>
            </TableRow>
          ))}
          {shown.length === 0 && (
            <TableRow>
              <TableCell
                colSpan={6}
                className="h-24 text-center text-muted-foreground"
              >
                No registries match “{search}”.
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </>,
  );
};
