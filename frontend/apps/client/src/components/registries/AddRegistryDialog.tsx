// Owner: U7 (publish). Used by the Registries page and the projects list
// ("Add a registry"). Props are a cross-unit contract (CONTRACTS.md).
// Foundation stub.
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';
import type { RegistryItem } from '@/types/contracts';

export interface AddRegistryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onAdded?: (registry: RegistryItem) => void;
}

export const AddRegistryDialog = ({
  open,
  onOpenChange,
}: AddRegistryDialogProps) => (
  <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Add a registry</DialogTitle>
      </DialogHeader>
    </DialogContent>
  </Dialog>
);
