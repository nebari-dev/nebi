// Owner: U3 (newversion). Confirms leaving the new version page with
// unsaved changes.
import { Button } from '@nebi/ui/components/ui/button';
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';

export function DiscardNewVersionDialog({
  open,
  onOpenChange,
  baseVersion,
  onDiscard,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  baseVersion: number;
  onDiscard: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent role="alertdialog" className="max-w-[520px]">
        <DialogHeader>
          <DialogTitle>Discard new version?</DialogTitle>
        </DialogHeader>
        <p className="text-muted-foreground-strong text-sm">
          Your changes are discarded. Version {baseVersion} stays as it is.
        </p>
        <DialogFooter>
          <DialogClose render={<Button variant="secondary" />}>
            Go back
          </DialogClose>
          <Button variant="destructive" onClick={onDiscard}>
            Discard
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
