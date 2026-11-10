// Owner: U1 (projects). "Delete <project>?" confirmation (a stub in the
// prototype, built like its Uninstall confirmation) that submits the delete
// job and follows it with a progress toast.
import { Button } from '@nebi/ui/components/ui/button';
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';
import { toast } from '@nebi/ui/components/ui/toast';
import { useNavigate } from 'react-router-dom';
import { clientApiError } from '@/api/client/base';
import { trackJobToast } from '@/hooks/client/jobs';
import { useDeleteProject } from '@/hooks/client/projects';
import type { ProjectListItem } from '@/types/contracts';

/** What the delete removes; the environment is named only when installed. */
const removedText = (versions: number, installed: boolean) => {
  const parts = ['The project'];
  if (versions === 1) parts.push('its version');
  if (versions > 1) parts.push(`all ${versions} versions`);
  if (installed) parts.push('its installed environment');
  if (parts.length === 1) return 'The project is removed from this machine.';
  const last = parts.pop();
  return `${parts.join(', ')} and ${last} are removed from this machine.`;
};

export interface DeleteProjectDialogProps {
  project: ProjectListItem;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called once the delete job is submitted. */
  onDeleted?: () => void;
}

export const DeleteProjectDialog = ({
  project,
  open,
  onOpenChange,
  onDeleted,
}: DeleteProjectDialogProps) => {
  const navigate = useNavigate();
  const remove = useDeleteProject();
  const versions = project.latest_version ?? 0;

  const confirm = () =>
    remove.mutate(project.id, {
      onSuccess: (job) => {
        onOpenChange(false);
        onDeleted?.();
        trackJobToast(job, {
          title: `Deleting ${project.name}`,
          successTitle: `${project.name} deleted`,
          successDescription: 'It is no longer on this machine.',
          errorTitle: `${project.name} couldn’t be deleted`,
          onViewJob: (j) => navigate(`/jobs/${j.id}`),
        });
      },
      onError: (err) => {
        onOpenChange(false);
        toast.add({
          type: 'error',
          title: `${project.name} couldn’t be deleted`,
          description: clientApiError(err)?.error,
        });
      },
    });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[520px]" role="alertdialog">
        <DialogHeader>
          <DialogTitle>Delete {project.name}?</DialogTitle>
          {project.size && (
            <DialogDescription>
              Frees {project.size} on this machine.
            </DialogDescription>
          )}
        </DialogHeader>
        <p className="text-foreground text-sm">
          {removedText(versions, project.installed_version != null)} Copies on
          your server or in a registry stay.
        </p>
        <DialogFooter>
          <DialogClose render={<Button variant="secondary" />}>
            Cancel
          </DialogClose>
          <Button
            variant="destructive"
            loading={remove.isPending}
            onClick={confirm}
          >
            Delete project
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
