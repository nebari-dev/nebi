// Owner: U2 (detail). "Pull version N" for server-only versions on the
// version toolbar. "Push to server" is the remote unit's (U5)
// PushToServerButton, which also covers the first push of a project that
// is not on the server yet.
import { Button } from '@nebi/ui/components/ui/button';
import { toast } from '@nebi/ui/components/ui/toast';
import { Download } from 'lucide-react';
import { clientApiError } from '@/api/client/base';
import { usePullVersion } from '@/hooks/client/remote';
import type { ProjectListItem, VersionItem } from '@/types/contracts';

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
