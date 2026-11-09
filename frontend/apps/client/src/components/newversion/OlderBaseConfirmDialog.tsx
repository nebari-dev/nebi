// Owner: U3 (newversion). Asked when "Create new version" is used on a
// version older than the latest one (prototype screen OlderBaseConfirm).
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
import { versionList } from './changes';

export interface OlderBaseConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The version the user picked. */
  baseVersion: number;
  /** The newest local version. */
  latestVersion: number;
  /** Local versions newer than the base, ascending. */
  newerVersions: number[];
  /** Called with the chosen base version. */
  onStart: (from: number) => void;
}

export function OlderBaseConfirmDialog({
  open,
  onOpenChange,
  baseVersion,
  latestVersion,
  newerVersions,
  onStart,
}: OlderBaseConfirmDialogProps) {
  const newer = newerVersions.length ? newerVersions : [latestVersion];
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[520px]" showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>
            Start a new version from version {baseVersion}?
          </DialogTitle>
          <DialogDescription>
            Version {baseVersion} is older than the latest version, v
            {latestVersion}.
          </DialogDescription>
        </DialogHeader>
        <p className="text-foreground text-sm">
          The new version starts with version {baseVersion}’s packages and
          settings. Changes made in {versionList(newer)} won’t carry over, but
          those versions stay in the history.
        </p>
        <DialogFooter>
          <DialogClose
            render={<Button variant="ghost" className="sm:mr-auto" />}
          >
            Cancel
          </DialogClose>
          <Button variant="secondary" onClick={() => onStart(latestVersion)}>
            Start from v{latestVersion}
          </Button>
          <Button onClick={() => onStart(baseVersion)}>
            Start from version {baseVersion}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
