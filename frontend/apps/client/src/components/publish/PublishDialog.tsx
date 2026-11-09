// Owner: U7 (publish). Opened from the version rail (U2). Props are a
// cross-unit contract (CONTRACTS.md). Foundation stub.
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';
import type { VersionItem } from '@/types/contracts';

export interface PublishDialogProps {
  projectId: string;
  projectName: string;
  version: VersionItem;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const PublishDialog = ({
  version,
  open,
  onOpenChange,
}: PublishDialogProps) => (
  <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Publish version {version.number}</DialogTitle>
      </DialogHeader>
    </DialogContent>
  </Dialog>
);
