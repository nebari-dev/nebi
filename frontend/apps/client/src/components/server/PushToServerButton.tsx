// Owner: U5 (remote). "Push to server" for the project detail toolbar
// (prototype InSync): pushes every version not yet on the server, oldest
// first, up to the selected version. Shows "Pushing…" while it runs and
// toasts "<versions> pushed" + "This machine and the server are in sync."
// Renders nothing when the version is already on the server, is a
// server-only version, or no server is connected.
//
// U2 mounts it in the version toolbar:
//   <PushToServerButton project={project} version={version} versions={versions} />
import { Button } from '@nebi/ui/components/ui/button';
import { toast } from '@nebi/ui/components/ui/toast';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@nebi/ui/components/ui/tooltip';
import { SquareArrowUp } from 'lucide-react';
import { clientApiError } from '@/api/client/base';
import { usePushProject, useServerConnection } from '@/hooks/client/remote';
import type { ProjectListItem, VersionItem } from '@/types/contracts';
import { capitalize, versionList } from './format';
import { canPush, versionsToPush } from './pushPlan';

export interface PushToServerButtonProps {
  project: ProjectListItem;
  version: VersionItem;
  /** The project's version list (for the "Pushes versions …" tooltip). */
  versions?: VersionItem[];
}

export const PushToServerButton = ({
  project,
  version,
  versions,
}: PushToServerButtonProps) => {
  const { data: connection } = useServerConnection();
  const push = usePushProject();
  if (!canPush(project, version, !!connection?.connected)) return null;

  const numbers = versionsToPush(project, versions, version.number);
  const onClick = () =>
    push.mutate(
      { projectId: project.id, upTo: version.number },
      {
        onSuccess: (res) => {
          const pushed = res.pushed_versions;
          const remaining = (versions ?? [])
            .filter((v) => !v.server_only && v.number > version.number)
            .filter((v) => v.number > res.server_latest_version)
            .map((v) => v.number)
            .sort((a, b) => a - b);
          const description =
            remaining.length > 0
              ? `${capitalize(versionList(remaining))} ${remaining.length === 1 ? 'is' : 'are'} still only on this machine.`
              : 'This machine and the server are in sync.';
          toast.add({
            type: 'success',
            title:
              pushed.length > 0
                ? `${capitalize(versionList(pushed))} pushed`
                : 'Already on the server',
            description,
          });
        },
        onError: (err) =>
          toast.add({
            type: 'error',
            title: 'Couldn’t push to the server',
            description: clientApiError(err)?.error ?? String(err),
          }),
      },
    );

  const button = (
    <Button
      variant="secondary"
      size="sm"
      loading={push.isPending}
      loadingText="Pushing…"
      onClick={onClick}
      data-tour="push"
    >
      <SquareArrowUp />
      Push to server
    </Button>
  );
  if (numbers.length <= 1) return button;
  return (
    <Tooltip>
      <TooltipTrigger render={button} />
      <TooltipContent>Pushes {versionList(numbers)}</TooltipContent>
    </Tooltip>
  );
};
