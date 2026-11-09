// Owner: U2 (detail). "Uninstall version N?" confirmation, used by the
// project detail page and the projects list (U1). Props are a cross-unit
// contract (CONTRACTS.md). Submits the uninstall job and follows it with
// the shared job toast.
import { ConfirmDialog } from '@nebi/ui/components/confirm-dialog';
import { toast } from '@nebi/ui/components/ui/toast';
import { useNavigate } from 'react-router-dom';
import { clientApiError } from '@/api/client/base';
import { useUninstall } from '@/hooks/client/detail';
import { trackJobToast } from '@/hooks/client/jobs';
import type { ProjectListItem } from '@/types/contracts';

export interface UninstallDialogProps {
  project: ProjectListItem;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const UninstallDialog = ({
  project,
  open,
  onOpenChange,
}: UninstallDialogProps) => {
  const navigate = useNavigate();
  const uninstall = useUninstall();
  const version = project.installed_version;
  const versionLabel = version != null ? `version ${version}` : 'this project';
  const title = version != null ? `Version ${version}` : project.name;

  const confirm = () => {
    uninstall.mutate(project.id, {
      onSuccess: (job) => {
        onOpenChange(false);
        trackJobToast(job, {
          title: `Uninstalling ${versionLabel}`,
          successTitle: `${title} uninstalled`,
          successDescription: project.size
            ? `Freed ${project.size} on this machine.`
            : undefined,
          errorTitle: `${title} couldn’t uninstall`,
          onViewJob: (j) => navigate(`/jobs/${j.id}`),
        });
      },
      onError: (err) => {
        onOpenChange(false);
        toast.add({
          type: 'error',
          title: `${title} couldn’t uninstall`,
          description:
            clientApiError(err)?.error ?? 'The uninstall could not start.',
        });
      },
    });
  };

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      onConfirm={confirm}
      keepOpenOnConfirm
      confirmLoading={uninstall.isPending}
      className="max-w-[520px]"
      title={`Uninstall ${versionLabel}?`}
      description={
        project.size ? `Frees ${project.size} on this machine.` : undefined
      }
      confirmText="Uninstall"
    >
      <p className="text-foreground text-sm">
        The environment is removed from disk. The project and all its versions
        stay
        {version != null
          ? `, and Nebi remembers version ${version} so you can install it again.`
          : '.'}
      </p>
    </ConfirmDialog>
  );
};
