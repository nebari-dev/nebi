// Owner: U3 (newversion). "Create new version" for a version's toolbar on
// the project detail page (U2 renders it). Disabled for server-only
// versions ("Pull version N first"); asks before starting from an older
// version.
import { Button } from '@nebi/ui/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@nebi/ui/components/ui/tooltip';
import { Plus } from 'lucide-react';
import type { VersionItem } from '@/types/contracts';
import { useStartNewVersion } from './useStartNewVersion';

export interface CreateNewVersionButtonProps {
  projectId: string;
  /** The version shown on the page (the base of the new version). */
  version: VersionItem;
  /** All versions of the project (the version rail). */
  versions: VersionItem[];
}

export function CreateNewVersionButton({
  projectId,
  version,
  versions,
}: CreateNewVersionButtonProps) {
  const { start, dialog } = useStartNewVersion(projectId, versions);
  const serverOnly = version.server_only;
  return (
    <>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant="secondary"
              size="sm"
              disabled={serverOnly}
              onClick={() => start(version.number)}
              data-tour="create-version"
            />
          }
        >
          <Plus />
          Create new version
        </TooltipTrigger>
        <TooltipContent>
          {serverOnly
            ? `Pull version ${version.number} first`
            : `Creates a new version based on version ${version.number}`}
        </TooltipContent>
      </Tooltip>
      {dialog}
    </>
  );
}
