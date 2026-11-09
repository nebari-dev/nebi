// Owner: U1 (projects). One row of the projects list (prototype
// ProjectsList): name, version (with "Update available"), environment
// (Install / Uninstall, "Install failed"), size, connected remotes and the
// row menu (Create new version, Delete project).
import { Badge } from '@nebi/ui/components/ui/badge';
import { Button } from '@nebi/ui/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
import { Spinner } from '@nebi/ui/components/ui/spinner';
import { TableCell, TableRow } from '@nebi/ui/components/ui/table';
import { toast } from '@nebi/ui/components/ui/toast';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@nebi/ui/components/ui/tooltip';
import {
  Ellipsis,
  Pencil,
  RotateCcwClock,
  Trash,
  TriangleAlert,
} from 'lucide-react';
import type { MouseEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { clientApiError } from '@/api/client/base';
import { useInstall } from '@/hooks/client/detail';
import { trackJobToast } from '@/hooks/client/jobs';
import type { ProjectListItem } from '@/types/contracts';
import { installTarget } from './projectList';

/** Placeholder for empty cells (en dash). */
export const EMPTY_CELL = '–';

const INTERACTIVE =
  'a, button, input, [role="menuitem"], [role="menu"], [data-slot="badge"]';

export interface ProjectRowProps {
  project: ProjectListItem;
  onUninstall: (project: ProjectListItem) => void;
  onDelete: (project: ProjectListItem) => void;
}

export const ProjectRow = ({
  project: p,
  onUninstall,
  onDelete,
}: ProjectRowProps) => {
  const navigate = useNavigate();
  const detailPath = `/projects/${p.id}`;

  const onRowClick = (event: MouseEvent<HTMLTableRowElement>) => {
    const target = event.target as HTMLElement;
    if (target.closest(INTERACTIVE)) return;
    if (!event.currentTarget.contains(target)) return;
    if (window.getSelection()?.toString()) return;
    navigate(detailPath);
  };

  return (
    <TableRow className="cursor-pointer" onClick={onRowClick}>
      <TableCell className="h-10 py-2">
        <Link
          to={detailPath}
          className="underline-offset-4 hover:underline focus-visible:underline"
        >
          {p.name}
        </Link>
      </TableCell>
      <TableCell className="h-10 py-2">
        <VersionCell project={p} />
      </TableCell>
      <TableCell className="h-10 py-2">
        <EnvironmentCell project={p} onUninstall={() => onUninstall(p)} />
      </TableCell>
      <TableCell className="h-10 py-2">{p.size || EMPTY_CELL}</TableCell>
      <TableCell className="h-10 py-1">
        {p.remotes.length === 0 ? (
          EMPTY_CELL
        ) : (
          <span className="flex flex-wrap gap-1">
            {p.remotes.map((r) => (
              <Button
                key={`${r.kind}:${r.name}`}
                variant="ghost"
                size="xs"
                title={r.url}
                onClick={() =>
                  navigate(r.kind === 'server' ? '/server' : '/registries')
                }
              >
                {r.name}
              </Button>
            ))}
          </span>
        )}
      </TableCell>
      <TableCell className="h-10 py-1">
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger
            variant="ghost"
            className="size-7 px-0"
            aria-label={`Actions for ${p.name}`}
          >
            <Ellipsis />
          </DropdownMenuTrigger>
          <DropdownMenuPortal>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                disabled={p.latest_version == null}
                onClick={() =>
                  navigate(
                    `/projects/${p.id}/new-version?from=${p.latest_version}`,
                  )
                }
              >
                <Pencil />
                Create new version
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={p.active_job_id != null}
                onClick={() => onDelete(p)}
              >
                <Trash />
                Delete project…
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenuPortal>
        </DropdownMenu>
      </TableCell>
    </TableRow>
  );
};

const VersionCell = ({ project: p }: { project: ProjectListItem }) => {
  const navigate = useNavigate();
  const shown = installTarget(p);
  return (
    <span className="flex items-center gap-1.5">
      {p.installed_version != null ? (
        <span>v{p.installed_version}</span>
      ) : shown == null ? (
        <span className="text-muted-foreground">{EMPTY_CELL}</span>
      ) : p.last_installed_version == null ? (
        <span className="text-muted-foreground">v{shown}</span>
      ) : (
        <>
          <span className="text-muted-foreground">
            v{p.last_installed_version}
          </span>
          <Tooltip>
            <TooltipTrigger
              aria-label="Last installed version"
              render={
                <button
                  type="button"
                  className="rounded-sm text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
                />
              }
            >
              <RotateCcwClock className="size-3.5" aria-hidden />
            </TooltipTrigger>
            <TooltipContent side="bottom">
              Last installed version. This project isn’t installed right now.
            </TooltipContent>
          </Tooltip>
        </>
      )}
      {p.update_available && (
        <Badge
          variant="info"
          render={
            <button
              type="button"
              onClick={() => navigate(`/projects/${p.id}`)}
            />
          }
        >
          Update available
        </Badge>
      )}
    </span>
  );
};

const EnvironmentCell = ({
  project: p,
  onUninstall,
}: {
  project: ProjectListItem;
  onUninstall: () => void;
}) => {
  const navigate = useNavigate();
  const install = useInstall();
  const target = installTarget(p);

  if (p.status === 'pending' || p.status === 'creating') {
    return <Busy label="Creating…" />;
  }
  if (p.status === 'deleting') return <Busy label="Deleting…" />;
  if (p.status === 'failed' && p.latest_version == null) {
    return (
      <Badge variant="destructive">
        <TriangleAlert />
        Create failed
      </Badge>
    );
  }
  if (p.installed_version != null || p.install_status === 'uninstalling') {
    return (
      <Button
        variant="secondary"
        size="xs"
        loading={p.install_status === 'uninstalling'}
        loadingText="Uninstalling…"
        disabled={p.active_job_id != null}
        onClick={onUninstall}
      >
        Uninstall
      </Button>
    );
  }

  const startInstall = () =>
    install.mutate(
      { projectId: p.id },
      {
        onSuccess: (job) =>
          trackJobToast(job, {
            title: `Installing version ${job.version_number ?? target}`,
            successTitle: (j) =>
              `Version ${j.version_number ?? target} installed`,
            successDescription: 'Ready to use on this machine.',
            errorTitle: `Version ${job.version_number ?? target} couldn’t install`,
            onViewJob: (j) => navigate(`/jobs/${j.id}`),
          }),
        onError: (err) =>
          toast.add({
            type: 'error',
            title: `Version ${target} couldn’t install`,
            description: clientApiError(err)?.error,
          }),
      },
    );
  const installing = install.isPending || p.install_status === 'installing';

  return (
    <span className="flex items-center gap-1.5">
      <Button
        size="xs"
        loading={installing}
        loadingText="Installing…"
        disabled={target == null || (p.active_job_id != null && !installing)}
        onClick={startInstall}
      >
        Install
      </Button>
      {p.install_status === 'install_failed' &&
        (p.failed_install_job_id ? (
          <Badge
            variant="destructive"
            render={<Link to={`/jobs/${p.failed_install_job_id}`} />}
          >
            <TriangleAlert />
            Install failed
          </Badge>
        ) : (
          <Badge variant="destructive">
            <TriangleAlert />
            Install failed
          </Badge>
        ))}
    </span>
  );
};

const Busy = ({ label }: { label: string }) => (
  <span className="flex items-center gap-1.5 text-muted-foreground text-xs">
    <Spinner className="size-3.5" />
    {label}
  </span>
);
