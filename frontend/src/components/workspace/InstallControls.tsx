import { HardDriveDownload, Loader2, PackageMinus, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  useInstallWorkspace,
  useUninstallWorkspace,
} from '@/hooks/useWorkspaces';
import type { InstallStatus, Job } from '@/types';

interface InstallControlsProps {
  workspaceId: string;
  installStatus?: InstallStatus;
  // 'labeled' renders outline buttons with icon + text (detail page header);
  // 'icon' renders ghost icon-only buttons matching other table row actions.
  appearance?: 'labeled' | 'icon';
  // Called with the queued job right after an install or uninstall is
  // accepted, so callers can jump to the job's logs.
  onStarted?: (job: Job) => void;
}

// Install/Uninstall action for a workspace's environment. Renders nothing
// when install_status is absent (team-mode servers never install).
export const InstallControls = ({
  workspaceId,
  installStatus,
  appearance = 'labeled',
  onStarted,
}: InstallControlsProps) => {
  const installMutation = useInstallWorkspace(workspaceId);
  const uninstallMutation = useUninstallWorkspace(workspaceId);
  const [confirmingUninstall, setConfirmingUninstall] = useState(false);

  if (!installStatus) return null;

  const stop = (e: React.MouseEvent) => e.stopPropagation();

  const iconOnly = appearance === 'icon';
  const variant = iconOnly ? ('ghost' as const) : ('outline' as const);
  const size = iconOnly ? ('icon' as const) : ('sm' as const);
  const iconClass = iconOnly ? 'h-4 w-4' : 'h-4 w-4 mr-1.5';

  if (installStatus === 'installing' || installStatus === 'uninstalling') {
    const label =
      installStatus === 'installing' ? 'Installing...' : 'Uninstalling...';
    return (
      <Button
        variant={variant}
        size={size}
        disabled
        onClick={stop}
        aria-label={label}
        title={label}
      >
        <Loader2 className={`${iconClass} animate-spin`} />
        {!iconOnly && label}
      </Button>
    );
  }

  if (installStatus === 'installed') {
    return (
      <>
        <Button
          variant={variant}
          size={size}
          disabled={uninstallMutation.isPending}
          onClick={(e) => {
            stop(e);
            setConfirmingUninstall(true);
          }}
          aria-label="Uninstall environment"
          title="Remove the installed environment (.pixi/envs); pixi.toml and pixi.lock are kept"
        >
          {iconOnly ? (
            <PackageMinus className={iconClass} />
          ) : (
            <>
              <Trash2 className={iconClass} />
              Uninstall
            </>
          )}
        </Button>
        <ConfirmDialog
          open={confirmingUninstall}
          onOpenChange={setConfirmingUninstall}
          onConfirm={() =>
            uninstallMutation.mutate(undefined, { onSuccess: onStarted })
          }
          title="Uninstall environment?"
          description="This removes the installed environment (.pixi/envs). pixi.toml and pixi.lock are kept, so you can reinstall later."
          confirmText="Uninstall"
          variant="destructive"
        />
      </>
    );
  }

  // not_installed or install_failed
  const installLabel =
    installStatus === 'install_failed' ? 'Retry Install' : 'Install';
  return (
    <Button
      variant={variant}
      size={size}
      disabled={installMutation.isPending}
      onClick={(e) => {
        stop(e);
        installMutation.mutate(undefined, { onSuccess: onStarted });
      }}
      aria-label={
        installStatus === 'install_failed'
          ? 'Retry environment install'
          : 'Install environment'
      }
      title="Download and install packages from the lockfile"
    >
      <HardDriveDownload className={iconClass} />
      {!iconOnly && installLabel}
    </Button>
  );
};
