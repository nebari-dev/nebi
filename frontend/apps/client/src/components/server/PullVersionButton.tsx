// Owner: U5 (remote). "Pull version N" for a server-only version in the
// project detail toolbar (prototype ServerAhead). Renders nothing for
// versions already on this machine.
//
// U2 mounts it in the version toolbar:
//   <PullVersionButton projectId={project.id} version={version} />
import { Button } from '@nebi/ui/components/ui/button';
import { toast } from '@nebi/ui/components/ui/toast';
import { Download } from 'lucide-react';
import { clientApiError } from '@/api/client/base';
import { usePullVersion } from '@/hooks/client/remote';
import type { VersionItem } from '@/types/contracts';

export interface PullVersionButtonProps {
  projectId: string;
  version: VersionItem;
  onPulled?: (version: VersionItem) => void;
}

export const PullVersionButton = ({
  projectId,
  version,
  onPulled,
}: PullVersionButtonProps) => {
  const pull = usePullVersion();
  if (!version.server_only) return null;
  return (
    <Button
      size="sm"
      loading={pull.isPending}
      loadingText="Pulling…"
      onClick={() =>
        pull.mutate(
          { projectId, n: version.number },
          {
            onSuccess: (v) => {
              toast.add({
                type: 'success',
                title: `Version ${v.number} pulled`,
                description: 'It is on this machine now. Install it to use it.',
              });
              onPulled?.(v);
            },
            onError: (err) =>
              toast.add({
                type: 'error',
                title: `Version ${version.number} couldn’t be pulled`,
                description: clientApiError(err)?.error ?? String(err),
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
