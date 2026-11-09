// Owner: U6 (sharing). Opened from the project detail page (U2, "Share")
// and the Server page row menu (U5, "Manage access"). Props are a
// cross-unit contract (CONTRACTS.md). Foundation stub.
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';

export interface ShareDialogProps {
  serverProjectId: string;
  projectName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const ShareDialog = ({
  projectName,
  open,
  onOpenChange,
}: ShareDialogProps) => (
  <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Share {projectName}</DialogTitle>
      </DialogHeader>
    </DialogContent>
  </Dialog>
);
