// Owner: U5 (remote). The Server page (prototype ServerConnected,
// ServerDisconnected, ConnectServer, ManageConnection, DisconnectConfirm,
// PullAndInstall, PullToast). Deep links: ?dialog=connect|manage and
// ?dialog=pull|pull-install|share&project=<server project id>.
import { Button } from '@nebi/ui/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@nebi/ui/components/ui/tooltip';
import { RefreshCw, Server, TriangleAlert } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { clientApiError } from '@/api/client/base';
import { ConnectServerDialog } from '@/components/server/ConnectServerDialog';
import { displayServerUrl, syncedAgo } from '@/components/server/format';
import { PullDialog } from '@/components/server/PullDialog';
import { trackPullToast } from '@/components/server/pullToast';
import { RemoteDetailsDialog } from '@/components/server/RemoteDetailsDialog';
import { ServerProjectActions } from '@/components/server/ServerProjectActions';
import { ServerProjectsTable } from '@/components/server/ServerProjectsTable';
import { ShareDialog } from '@/components/share/ShareDialog';
import { useProjects } from '@/hooks/client/projects';
import {
  remoteErrorKind,
  useRefreshServerProjects,
  useServerConnection,
  useServerProjects,
} from '@/hooks/client/remote';
import type { PullingState, ServerDialog } from '@/types/client/remote';
import type { ServerConnection } from '@/types/contracts';

export const ServerPage = () => {
  const { data: connection, isLoading } = useServerConnection();
  const [dialog, setDialog] = useState<ServerDialog | null>(null);
  const [params, setParams] = useSearchParams();
  const connected = !!connection?.connected;
  const projectsQuery = useServerProjects('', connected);

  // Deep links (prototype ?dialog=… query parameters).
  useEffect(() => {
    const kind = params.get('dialog');
    if (!kind || isLoading) return;
    if (kind === 'connect' || kind === 'manage') {
      setDialog({
        kind: connected && kind === 'manage' ? 'manage' : 'connect',
      });
      setParams({}, { replace: true });
      return;
    }
    const id = params.get('project');
    const project = projectsQuery.data?.projects.find((p) => p.id === id);
    if (!project) {
      if (!connected || projectsQuery.isFetched)
        setParams({}, { replace: true });
      return;
    }
    if (kind === 'pull' || kind === 'pull-install') {
      setDialog({ kind: 'pull', project, andInstall: kind === 'pull-install' });
    } else if (kind === 'share') {
      setDialog({ kind: 'share', project });
    }
    setParams({}, { replace: true });
  }, [
    params,
    setParams,
    isLoading,
    connected,
    projectsQuery.data,
    projectsQuery.isFetched,
  ]);

  const close = (open: boolean) => {
    if (!open) setDialog(null);
  };

  return (
    <main className="flex flex-col gap-5 px-12 py-12">
      {connected && connection ? (
        <ConnectedView connection={connection} onOpen={setDialog} />
      ) : (
        <>
          <div className="flex flex-col gap-2">
            <h1 className="font-bold text-3xl text-foreground">Server</h1>
            {!isLoading && (
              <p className="text-base text-muted-foreground">Not connected</p>
            )}
          </div>
          {!isLoading && (
            <section
              className="mt-20 flex flex-col items-center gap-3 text-center"
              aria-labelledby="server-empty-title"
            >
              <h2
                id="server-empty-title"
                className="font-semibold text-foreground text-xl leading-7"
              >
                No server connected
              </h2>
              <p className="max-w-xl text-muted-foreground-strong text-sm">
                Connect your team’s Nebi server to browse and pull shared
                projects.
              </p>
              <Button
                className="mt-2"
                onClick={() => setDialog({ kind: 'connect' })}
              >
                Connect server
              </Button>
            </section>
          )}
        </>
      )}
      {(dialog?.kind === 'connect' || dialog?.kind === 'manage') && (
        <ConnectServerDialog mode={dialog.kind} open onOpenChange={close} />
      )}
      {dialog?.kind === 'share' && (
        <ShareDialog
          serverProjectId={dialog.project.id}
          projectName={dialog.project.name}
          open
          onOpenChange={close}
        />
      )}
      {dialog?.kind === 'details' && (
        <RemoteDetailsDialog
          serverProjectId={dialog.project.id}
          open
          onOpenChange={close}
        />
      )}
      {dialog?.kind === 'pull' && <PullFlow dialog={dialog} onClose={close} />}
    </main>
  );
};

/** The pull dialog plus the pull job toast and the row's loading state. */
const PullFlow = ({
  dialog,
  onClose,
}: {
  dialog: Extract<ServerDialog, { kind: 'pull' }>;
  onClose: (open: boolean) => void;
}) => {
  const navigate = useNavigate();
  const { data: localProjects } = useProjects();
  return (
    <PullDialog
      project={dialog.project}
      andInstall={dialog.andInstall}
      open
      onOpenChange={onClose}
      localProjects={localProjects}
      onPullStarted={(res) => {
        pullingStore.set({
          serverProjectId: dialog.project.id,
          localProjectId: res.project_id,
          installing: false,
        });
        trackPullToast(res.job, {
          projectName: dialog.project.name,
          install: dialog.andInstall,
          onOpenProject: () => navigate(`/projects/${res.project_id}`),
          onViewJob: (job) => navigate(`/jobs/${job.id}`),
          onInstalling: () =>
            pullingStore.set({
              serverProjectId: dialog.project.id,
              localProjectId: res.project_id,
              installing: true,
            }),
          onSettled: () => pullingStore.set(null),
        });
      }}
    />
  );
};

// The pull in progress outlives the dialog (and the page, if the user
// navigates away and back), so it is a tiny module-level store.
const pullingStore = (() => {
  let value: PullingState | null = null;
  const listeners = new Set<(v: PullingState | null) => void>();
  return {
    get: () => value,
    set: (v: PullingState | null) => {
      value = v;
      for (const l of listeners) l(v);
    },
    subscribe: (l: (v: PullingState | null) => void) => {
      listeners.add(l);
      return () => {
        listeners.delete(l);
      };
    },
  };
})();

const usePulling = () => {
  const [value, setValue] = useState(pullingStore.get());
  useEffect(() => pullingStore.subscribe(setValue), []);
  return value;
};

/** Re-renders every 30 s so "Last synced" stays current. */
const useNow = () => {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(t);
  }, []);
  return now;
};

const ConnectedView = ({
  connection,
  onOpen,
}: {
  connection: ServerConnection;
  onOpen: (d: ServerDialog) => void;
}) => {
  const projects = useServerProjects('');
  const refresh = useRefreshServerProjects();
  const pulling = usePulling();
  const now = useNow();
  const shownUrl = displayServerUrl(connection.url);
  const syncing = refresh.isPending || (projects.isFetching && !projects.data);
  const lastSynced =
    projects.data?.last_synced_at ?? connection.last_synced_at ?? null;
  const errorKind = projects.isError ? remoteErrorKind(projects.error) : null;

  return (
    <>
      <div className="flex items-center justify-between gap-4">
        <div className="flex flex-col gap-3">
          <h1 className="font-bold text-3xl text-foreground">Server</h1>
          <div className="flex flex-col gap-1">
            <span className="font-medium text-muted-foreground text-sm">
              Connected to
            </span>
            <span className="flex items-center gap-2 font-semibold text-base text-foreground leading-5">
              <Server className="size-4" aria-hidden />
              {shownUrl}
            </span>
          </div>
        </div>
        <Button variant="outline" onClick={() => onOpen({ kind: 'manage' })}>
          Manage connection
        </Button>
      </div>
      <dl className="flex gap-10">
        <div className="flex flex-col gap-0.5">
          <dt className="font-medium text-foreground text-sm leading-6">
            Signed in as
          </dt>
          <dd className="text-foreground text-sm">{connection.username}</dd>
        </div>
        <div className="flex flex-col gap-0.5">
          <dt className="flex items-center gap-2.5 font-medium text-foreground text-sm">
            Last synced
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="secondary"
                    size="icon-xs"
                    onClick={() => refresh.mutate('')}
                    disabled={syncing}
                    aria-label="Refresh the server’s project list"
                  />
                }
              >
                <RefreshCw
                  className={syncing ? 'motion-safe:animate-spin' : undefined}
                />
              </TooltipTrigger>
              <TooltipContent>Refresh</TooltipContent>
            </Tooltip>
          </dt>
          <dd className="text-foreground text-sm" aria-live="polite">
            {syncing ? 'Syncing…' : syncedAgo(lastSynced, now)}
          </dd>
        </div>
      </dl>
      {errorKind && errorKind !== 'not_connected' ? (
        <ServerProblem
          kind={errorKind}
          url={shownUrl}
          message={clientApiError(refresh.error ?? projects.error)?.error}
          retrying={projects.isFetching}
          onRetry={() => projects.refetch()}
          onManage={() => onOpen({ kind: 'manage' })}
        />
      ) : (
        <ServerProjectsTable
          projects={projects.data?.projects ?? []}
          actionsWidth="w-[180px]"
          actions={(p) => (
            <ServerProjectActions
              project={p}
              pulling={pulling}
              onPull={(andInstall) =>
                onOpen({ kind: 'pull', project: p, andInstall })
              }
              onManageAccess={() => onOpen({ kind: 'share', project: p })}
              onDetails={() => onOpen({ kind: 'details', project: p })}
            />
          )}
        />
      )}
    </>
  );
};

const ServerProblem = ({
  kind,
  url,
  message,
  retrying,
  onRetry,
  onManage,
}: {
  kind: ReturnType<typeof remoteErrorKind>;
  url: string;
  message?: string;
  retrying: boolean;
  onRetry: () => void;
  onManage: () => void;
}) => {
  const auth = kind === 'auth' || kind === 'forbidden';
  return (
    <section
      role="alert"
      className="mt-10 flex flex-col items-center gap-3 text-center"
    >
      <TriangleAlert className="size-6 text-warning-foreground" aria-hidden />
      <h2 className="font-semibold text-foreground text-xl leading-7">
        {auth ? `${url} didn’t accept your sign-in` : `Can’t reach ${url}`}
      </h2>
      <p className="max-w-xl text-muted-foreground-strong text-sm">
        {auth
          ? 'Your token may have expired. Update it in Manage connection.'
          : 'Check your network or the server address. Your projects on this machine keep working, and Nebi keeps trying in the background.'}
      </p>
      {message && kind === 'other' && (
        <p className="max-w-xl text-muted-foreground text-xs">{message}</p>
      )}
      <div className="mt-2 flex gap-2">
        {auth ? (
          <Button onClick={onManage}>Manage connection</Button>
        ) : (
          <Button
            variant="secondary"
            onClick={onRetry}
            loading={retrying}
            loadingText="Trying…"
          >
            Try again
          </Button>
        )}
      </div>
    </section>
  );
};
