// Owner: U5 (remote). Used by the Server page and the projects list empty
// state ("Connect to server"). Props are a cross-unit contract
// (CONTRACTS.md). Foundation stub.
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';
import type { ServerConnection } from '@/types/contracts';

export interface ConnectServerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConnected?: (connection: ServerConnection) => void;
}

export const ConnectServerDialog = ({
  open,
  onOpenChange,
}: ConnectServerDialogProps) => (
  <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Connect server</DialogTitle>
      </DialogHeader>
    </DialogContent>
  </Dialog>
);
