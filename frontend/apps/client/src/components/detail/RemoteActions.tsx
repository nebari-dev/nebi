// Owner: U2 (detail). Server actions on the version toolbar: "Push to
// server" for local versions the linked server lacks and "Pull version N"
// for server-only versions. The push/pull logic is the remote unit's (U5)
// hooks; this file only places the buttons and words the toasts.
import { Button } from '@nebi/ui/components/ui/button';
import { toast } from '@nebi/ui/components/ui/toast';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@nebi/ui/components/ui/tooltip';
import { Download, SquareArrowUp } from 'lucide-react';
import { clientApiError } from '@/api/client/base';
import { usePullVersion, usePushProject } from '@/hooks/client/remote';
import type { ProjectListItem, VersionItem } from '@/types/contracts';
import {
  capitalizeFirst,
  serverRemote,
  unpushedVersions,
  versionsPhrase,
} from './format';

export interface PushButtonProps {
  project: ProjectListItem;
  version: VersionItem;
  versions: VersionItem[];
}

/** "Push to server": shown on local versions newer than the server's. */
export const PushButton = ({ project, version, versions }: PushButtonProps) => {
  const push = usePushProject();
  if (!serverRemote(project) || version.server_only) return null;
  const pending = unpushedVersions(project, versions);
  if (!pending.includes(version.number)) return null;

  const onPush = () =>
    push.mutate(project.id, {
      onSuccess: (res) => {
        const remaining = versions
          .filter(
            (v) =>
              !v.server_only &&
              v.number > res.server_latest_version &&
              !res.pushed_versions.includes(v.number),
          )
          .map((v) => v.number)
          .sort((a, b) => a - b);
        if (res.pushed_versions.length === 0) {
          toast.add({
            type: 'info',
            title: 'Nothing to push',
            description: 'This machine and the server are in sync.',
          });
          return;
        }
        toast.add({
          type: 'success',
          title: `${capitalizeFirst(versionsPhrase(res.pushed_versions))} pushed`,
          description: remaining.length
            ? `${capitalizeFirst(versionsPhrase(remaining))} ${remaining.length === 1 ? 'is' : 'are'} still only on this machine.`
            : 'This machine and the server are in sync.',
        });
      },
      onError: (err) =>
        toast.add({
          type: 'error',
          title: 'Couldn’t push to the server',
          description: clientApiError(err)?.error ?? 'Try again in a moment.',
        }),
    });

  const button = (
    <Button
      variant="secondary"
      size="sm"
      loading={push.isPending}
      loadingText="Pushing…"
      onClick={onPush}
      data-tour="push"
    >
      <SquareArrowUp />
      Push to server
    </Button>
  );
  if (pending.length === 1) return button;
  return (
    <Tooltip>
      <TooltipTrigger render={button} />
      <TooltipContent>Pushes {versionsPhrase(pending)}</TooltipContent>
    </Tooltip>
  );
};

/** "Pull version N": brings a server-only version to this machine. */
export const PullVersionButton = ({
  project,
  version,
}: {
  project: ProjectListItem;
  version: VersionItem;
}) => {
  const pull = usePullVersion();
  return (
    <Button
      size="sm"
      loading={pull.isPending}
      loadingText="Pulling…"
      onClick={() =>
        pull.mutate(
          { projectId: project.id, n: version.number },
          {
            onSuccess: () =>
              toast.add({
                type: 'success',
                title: `Version ${version.number} pulled`,
                description: 'It is on this machine now. Install it to use it.',
              }),
            onError: (err) =>
              toast.add({
                type: 'error',
                title: `Couldn’t pull version ${version.number}`,
                description:
                  clientApiError(err)?.error ?? 'Try again in a moment.',
              }),
          },
        )
      }
    >
      <Download />
      Pull version {version.number}
    </Button>
  );
};
