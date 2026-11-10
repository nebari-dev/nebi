// Owner: U2 (detail). Project details (/projects/:id?v=N): header with the
// project path and Share, the version rail, and the selected version with
// its actions and environment specification. The first visit offers a
// guided tour (also under Help).
import { Badge } from '@nebi/ui/components/ui/badge';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@nebi/ui/components/ui/breadcrumb';
import { Button } from '@nebi/ui/components/ui/button';
import { Spinner } from '@nebi/ui/components/ui/spinner';
import { toast } from '@nebi/ui/components/ui/toast';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@nebi/ui/components/ui/tooltip';
import { Copy, Share2, TriangleAlert } from 'lucide-react';
import { useMemo, useState } from 'react';
import {
  Link,
  useNavigate,
  useParams,
  useSearchParams,
} from 'react-router-dom';
import { clientApiError, httpStatus } from '@/api/client/base';
import { pickVersion, serverRemote } from '@/components/detail/format';
import { VersionPanel } from '@/components/detail/VersionPanel';
import { VersionRail } from '@/components/detail/VersionRail';
import type { VersionFilterContext } from '@/components/detail/versionFilters';
import { ShareDialog } from '@/components/share/ShareDialog';
import { ProjectTour } from '@/components/tour/ProjectTour';
import {
  useInstallVersion,
  useProjectInstallState,
  useVersions,
} from '@/hooks/client/detail';
import { useProject } from '@/hooks/client/projects';
import type { ProjectListItem, VersionItem } from '@/types/contracts';

const mainClass =
  'flex h-[calc(100dvh-3.5rem)] min-h-[560px] flex-col gap-6 px-9 pt-9';

export const ProjectDetailPage = () => {
  const { id = '' } = useParams();
  const project = useProject(id);
  const versions = useVersions(id);

  if (project.isLoading) {
    return (
      <main className={mainClass}>
        <div className="flex items-center gap-2 text-muted-foreground text-sm">
          <Spinner />
          Loading project…
        </div>
      </main>
    );
  }
  if (!project.data) {
    const notFound = httpStatus(project.error) === 404;
    return (
      <main className="flex flex-col gap-3 p-9">
        <h1 className="font-bold text-3xl text-foreground">
          {notFound ? 'Project not found' : 'Couldn’t load the project'}
        </h1>
        {!notFound && (
          <p className="text-muted-foreground text-sm">
            {clientApiError(project.error)?.error ?? 'Try again in a moment.'}
          </p>
        )}
        <Link to="/" className="text-sm underline underline-offset-4">
          Back to projects
        </Link>
      </main>
    );
  }
  return (
    <ProjectDetail
      project={project.data}
      versions={versions.data}
      versionsLoading={versions.isLoading}
      versionsError={versions.error}
    />
  );
};

const ProjectDetail = ({
  project,
  versions,
  versionsLoading,
  versionsError,
}: {
  project: ProjectListItem;
  versions: VersionItem[] | undefined;
  versionsLoading: boolean;
  versionsError: unknown;
}) => {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [shareOpen, setShareOpen] = useState(false);
  const installState = useProjectInstallState(project, (jobId) =>
    navigate(`/jobs/${jobId}`),
  );
  const { install, isPending: installPending } = useInstallVersion(project);

  const list = versions ?? [];
  const selected = pickVersion(
    list,
    Number(params.get('v')),
    project.installed_version,
  );
  const remote = serverRemote(project);
  const filterContext = useMemo<VersionFilterContext>(
    () => ({
      linked: remote !== null,
      serverLatest: project.server_latest_version,
    }),
    [remote, project.server_latest_version],
  );
  const installFailed =
    project.install_status === 'install_failed' ||
    !!project.failed_install_job_id;

  const copyPath = async () => {
    try {
      await navigator.clipboard.writeText(project.path);
      toast.add({
        type: 'success',
        title: 'Path copied',
        description: project.path,
      });
    } catch {
      toast.add({
        type: 'error',
        title: 'Couldn’t copy the path',
        description: 'Your browser blocked clipboard access.',
      });
    }
  };

  return (
    <main className={mainClass}>
      <header className="flex flex-col gap-1.5">
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink render={<Link to="/" />}>Projects</BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{project.name}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
        <div className="flex items-center justify-between gap-4">
          <div className="flex min-w-0 items-center gap-2">
            <h1 className="truncate font-bold text-3xl text-foreground">
              {project.name}
            </h1>
            {installFailed && (
              <Badge variant="destructive">
                <TriangleAlert />
                Install failed
              </Badge>
            )}
          </div>
          {/* The tour's Share step only shows for a linked project. */}
          <Tooltip>
            <TooltipTrigger render={<span className="inline-flex" />}>
              <Button
                variant="secondary"
                size="sm"
                data-tour={remote?.server_project_id ? 'share' : undefined}
                disabled={!remote?.server_project_id}
                onClick={() => setShareOpen(true)}
              >
                <Share2 />
                Share
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {remote?.server_project_id
                ? 'Manage who can open this project on the server'
                : 'Push this project to a server to share it'}
            </TooltipContent>
          </Tooltip>
        </div>
        <div className="flex min-w-0 items-center gap-1.5 text-muted-foreground text-sm">
          <span className="truncate">{project.path}</span>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-xs"
                  onClick={copyPath}
                  aria-label="Copy path"
                />
              }
            >
              <Copy />
            </TooltipTrigger>
            <TooltipContent>Copy path</TooltipContent>
          </Tooltip>
        </div>
      </header>

      <div className="flex min-h-0 flex-1 items-stretch gap-4">
        <VersionRail
          versions={list}
          selected={selected?.number}
          installingVersion={installState.installing?.version ?? null}
          filterContext={filterContext}
          onSelect={(n) => setParams({ v: String(n) }, { replace: true })}
        />
        <div className="w-px shrink-0 bg-border" aria-hidden />
        <div className="-mr-9 -ml-1 flex min-w-0 flex-1 overflow-y-auto pr-9 pb-9 pl-1">
          {selected ? (
            <VersionPanel
              key={`${selected.number}-${selected.server_only}`}
              project={project}
              version={selected}
              versions={list}
              installState={installState}
              onInstall={install}
              installPending={installPending}
            />
          ) : (
            <EmptyVersions loading={versionsLoading} error={versionsError} />
          )}
        </div>
      </div>

      {remote?.server_project_id && (
        <ShareDialog
          serverProjectId={remote.server_project_id}
          projectName={project.name}
          open={shareOpen}
          onOpenChange={setShareOpen}
        />
      )}
      <ProjectTour />
    </main>
  );
};

const EmptyVersions = ({
  loading,
  error,
}: {
  loading: boolean;
  error: unknown;
}) => {
  if (loading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground text-sm">
        <Spinner />
        Loading versions…
      </div>
    );
  }
  if (error) {
    return (
      <p className="text-destructive-foreground text-sm" role="alert">
        Couldn’t load the versions.{' '}
        {clientApiError(error)?.error ?? 'Try again in a moment.'}
      </p>
    );
  }
  return (
    <p className="text-muted-foreground text-sm">
      No versions yet. The first version appears here once the project is
      created.
    </p>
  );
};
