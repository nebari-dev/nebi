// Owner: U2 (detail). "Uninstall version N?" confirmation, used by the
// project detail page and the projects list (U1). Props are a cross-unit
// contract (CONTRACTS.md). Foundation stub; U2 matches the prototype copy
// ("Frees <size> on this machine.") and submits the uninstall job.
import { ConfirmDialog } from '@nebi/ui/components/confirm-dialog';
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
}: UninstallDialogProps) => (
  <ConfirmDialog
    open={open}
    onOpenChange={onOpenChange}
    onConfirm={() => {}}
    title={`Uninstall version ${project.installed_version ?? ''}?`}
    confirmText="Uninstall"
    variant="destructive"
  />
);
