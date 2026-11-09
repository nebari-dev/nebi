// Owner: U5 (remote). "Disconnect from <server>?" (prototype
// DisconnectConfirm, opened from Manage connection).
import { Button } from '@nebi/ui/components/ui/button';
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';
import { toast } from '@nebi/ui/components/ui/toast';
import { clientApiError } from '@/api/client/base';
import { useDisconnectServer } from '@/hooks/client/remote';
import { DISCONNECT_COPY } from './copy';
import { displayServerUrl } from './format';

export interface DisconnectDialogProps {
  url: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDisconnected?: () => void;
}

export const DisconnectDialog = ({
  url,
  open,
  onOpenChange,
  onDisconnected,
}: DisconnectDialogProps) => {
  const disconnect = useDisconnectServer();
  const shown = displayServerUrl(url);

  const confirm = () =>
    disconnect.mutate(undefined, {
      onSuccess: () => {
        toast.add({
          title: `Disconnected from ${shown}`,
          description:
            'Projects you pulled stay on this machine and keep working.',
        });
        onOpenChange(false);
        onDisconnected?.();
      },
      onError: (err) =>
        toast.add({
          type: 'error',
          title: 'Couldn’t disconnect',
          description: clientApiError(err)?.error ?? String(err),
        }),
    });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        role="alertdialog"
        className="max-w-[520px]"
        showCloseButton={false}
      >
        <DialogHeader>
          <DialogTitle>Disconnect from {shown}?</DialogTitle>
        </DialogHeader>
        <p className="text-muted-foreground text-sm">{DISCONNECT_COPY}</p>
        <DialogFooter>
          <DialogClose render={<Button variant="secondary" />}>
            Cancel
          </DialogClose>
          <Button
            variant="destructive"
            onClick={confirm}
            loading={disconnect.isPending}
            loadingText="Disconnecting…"
          >
            Disconnect
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
