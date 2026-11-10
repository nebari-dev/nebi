// Owner: U2 (detail). Install progress, success and failure banners of the
// selected version, and the Install / Uninstall / Pull controls.
import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from '@nebi/ui/components/ui/alert';
import { Button } from '@nebi/ui/components/ui/button';
import { Spinner } from '@nebi/ui/components/ui/spinner';
import {
  CircleCheck,
  Import,
  PackageX,
  RotateCw,
  TriangleAlert,
  X,
} from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  type ProjectInstallState,
  useInstallProgressStore,
} from '@/hooks/client/detail';
import type { ProjectListItem, VersionItem } from '@/types/contracts';
import { installErrorText } from './format';
import { PullVersionButton } from './RemoteActions';
import { UninstallDialog } from './UninstallDialog';

const hintClass = 'text-muted-foreground-strong text-xs';

export interface InstallControlsProps {
  project: ProjectListItem;
  version: VersionItem;
  state: ProjectInstallState;
  onInstall: (version: number) => void;
  installPending: boolean;
}

/** Install / Uninstall / Pull for the selected version. */
export const InstallControls = ({
  project,
  version,
  state,
  onInstall,
  installPending,
}: InstallControlsProps) => {
  const [confirmUninstall, setConfirmUninstall] = useState(false);
  const installed = project.installed_version;

  if (version.server_only) {
    return <PullVersionButton project={project} version={version} />;
  }
  if (installed === version.number) {
    return (
      <div className="flex items-center gap-2">
        <Button
          variant="secondary"
          size="sm"
          loading={state.uninstalling}
          loadingText="Uninstalling…"
          disabled={state.busy && !state.uninstalling}
          onClick={() => setConfirmUninstall(true)}
        >
          <PackageX />
          Uninstall
        </Button>
        {project.size && (
          <span className={hintClass}>{project.size} on disk</span>
        )}
        <UninstallDialog
          project={project}
          open={confirmUninstall}
          onOpenChange={setConfirmUninstall}
        />
      </div>
    );
  }
  const isInstalling = state.installing?.version === version.number;
  return (
    <div className="flex items-center gap-2">
      <Button
        size="sm"
        loading={isInstalling || installPending}
        loadingText="Installing…"
        disabled={state.busy && !isInstalling}
        onClick={() => onInstall(version.number)}
        data-tour="install"
      >
        <Import />
        Install
      </Button>
      {installed != null && (
        <span className={hintClass}>
          Replaces{' '}
          <Link
            to={`?v=${installed}`}
            replace
            className="underline underline-offset-4 hover:text-primary"
          >
            v{installed}
          </Link>{' '}
          on disk
        </span>
      )}
    </div>
  );
};

/** Progress / success banner for the selected version. */
export const InstallBanner = ({
  project,
  version,
  state,
}: {
  project: ProjectListItem;
  version: VersionItem;
  state: ProjectInstallState;
}) => {
  const navigate = useNavigate();
  const dismissDone = useInstallProgressStore((s) => s.dismissDone);
  if (state.installing?.version === version.number) {
    const jobId = state.installing.jobId;
    return (
      <div
        role="status"
        className="flex min-h-12 items-center justify-between gap-3 rounded-md bg-info py-2 pr-4 pl-4 text-info-foreground"
      >
        <span className="flex items-center gap-2 font-medium text-sm">
          <Spinner size="sm" />
          Installing version {version.number} · downloading and linking packages
        </span>
        <Button
          variant="link"
          size="xs"
          className="text-info-foreground"
          onClick={() => navigate(`/jobs/${jobId}`)}
        >
          View log
        </Button>
      </div>
    );
  }
  const done = state.done;
  if (!done || done.version !== version.number) return null;
  return (
    <div
      role="status"
      className="flex min-h-12 items-center justify-between gap-3 rounded-md bg-success py-2 pr-2 pl-4 text-success-foreground"
    >
      <span className="flex items-center gap-2 font-medium text-sm">
        <CircleCheck className="size-4" aria-hidden />
        Version {version.number} installed · ready to use on this machine
      </span>
      <span className="flex items-center gap-2">
        <Button
          variant="link"
          size="xs"
          className="text-success-foreground"
          onClick={() => navigate(`/jobs/${done.jobId}`)}
        >
          View log
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          className="text-success-foreground"
          onClick={() => dismissDone(project.id)}
          aria-label="Dismiss"
        >
          <X />
        </Button>
      </span>
    </div>
  );
};

/** "Version N couldn't install" with View job / Try again. */
export const InstallFailedAlert = ({
  version,
  state,
  onRetry,
}: {
  version: VersionItem;
  state: ProjectInstallState;
  onRetry: (version: number) => void;
}) => {
  const failed = state.failed;
  if (!failed || failed.version !== version.number) return null;
  return (
    <Alert variant="destructive">
      <TriangleAlert aria-hidden />
      <AlertTitle>Version {version.number} couldn’t install</AlertTitle>
      <AlertDescription>
        {installErrorText(failed.error, version.number)}
      </AlertDescription>
      <AlertAction className="flex items-center gap-2">
        <Button
          variant="outline"
          size="xs"
          render={<Link to={`/jobs/${failed.jobId}`} />}
        >
          View job
        </Button>
        <Button
          variant="secondary"
          size="xs"
          disabled={state.busy}
          onClick={() => onRetry(version.number)}
        >
          <RotateCw />
          Try again
        </Button>
      </AlertAction>
    </Alert>
  );
};
