import { HardDriveDownload, Loader2, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  useInstallWorkspace,
  useUninstallWorkspace,
} from '@/hooks/useWorkspaces';
import type { InstallStatus, Job } from '@/types';

interface InstallControlsProps {
  workspaceId: string;
  installStatus?: InstallStatus;
  // Called with the queued job right after an install or uninstall is
  // accepted, so callers can jump to the job's logs.
  onStarted?: (job: Job) => void;
}

// Install/Uninstall action for a workspace's environment. Renders nothing
// when install_status is absent (team-mode servers never install).
export const InstallControls = ({
  workspaceId,
  installStatus,
  onStarted,
}: InstallControlsProps) => {
  const installMutation = useInstallWorkspace(workspaceId);
  const uninstallMutation = useUninstallWorkspace(workspaceId);

  if (!installStatus) return null;

  const stop = (e: React.MouseEvent) => e.stopPropagation();

  if (installStatus === 'installing' || installStatus === 'uninstalling') {
    return (
      <Button variant="outline" size="sm" disabled onClick={stop}>
        <Loader2 className="h-4 w-4 mr-1.5 animate-spin" />
        {installStatus === 'installing' ? 'Installing...' : 'Uninstalling...'}
      </Button>
    );
  }

  if (installStatus === 'installed') {
    return (
      <Button
        variant="outline"
        size="sm"
        disabled={uninstallMutation.isPending}
        onClick={(e) => {
          stop(e);
          uninstallMutation.mutate(undefined, { onSuccess: onStarted });
        }}
        aria-label="Uninstall environment"
        title="Remove the installed environment (.pixi/envs); pixi.toml and pixi.lock are kept"
      >
        <Trash2 className="h-4 w-4 mr-1.5" />
        Uninstall
      </Button>
    );
  }

  // not_installed or install_failed
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={installMutation.isPending}
      onClick={(e) => {
        stop(e);
        installMutation.mutate(undefined, { onSuccess: onStarted });
      }}
      aria-label="Install environment"
      title="Download and install packages from the lockfile"
    >
      <HardDriveDownload className="h-4 w-4 mr-1.5" />
      {installStatus === 'install_failed' ? 'Retry Install' : 'Install'}
    </Button>
  );
};
