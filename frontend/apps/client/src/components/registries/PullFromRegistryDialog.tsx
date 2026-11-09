// Owner: U7 (publish). Used by the projects list ("Pull from registry").
// Props are a cross-unit contract (CONTRACTS.md). Foundation stub.
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';

export interface PullFromRegistryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called with the new local project id once the pull job is submitted. */
  onPulled?: (projectId: string) => void;
}

export const PullFromRegistryDialog = ({
  open,
  onOpenChange,
}: PullFromRegistryDialogProps) => (
  <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Pull from registry</DialogTitle>
      </DialogHeader>
    </DialogContent>
  </Dialog>
);
