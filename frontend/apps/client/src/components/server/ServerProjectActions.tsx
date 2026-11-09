// Owner: U5 (remote). Row actions on the Server page: "Pull and install"
// (or "Open project" when it is already here, or "Pulling…"/"Installing…"
// while a pull runs) plus a menu with "Pull project" and "Manage access".
import { Button } from '@nebi/ui/components/ui/button';
import {
  ButtonGroup,
  ButtonGroupSeparator,
} from '@nebi/ui/components/ui/button-group';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
import { ChevronDown } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import type { PullingState } from '@/types/client/remote';
import type { ServerProjectItem } from '@/types/contracts';

export interface ServerProjectActionsProps {
  project: ServerProjectItem;
  pulling: PullingState | null;
  onPull: (andInstall: boolean) => void;
  onManageAccess: () => void;
  onDetails?: () => void;
}

export const ServerProjectActions = ({
  project,
  pulling,
  onPull,
  onManageAccess,
  onDetails,
}: ServerProjectActionsProps) => {
  const navigate = useNavigate();
  const isPulling = pulling?.serverProjectId === project.id;
  const localId = project.local_project_id;
  const busy = pulling != null;

  return (
    <ButtonGroup aria-label={`Actions for ${project.name}`}>
      {isPulling ? (
        <Button
          variant="outline"
          loading
          loadingText={pulling?.installing ? 'Installing…' : 'Pulling…'}
          className="min-w-[117px]"
        />
      ) : localId ? (
        <Button
          variant="outline"
          className="min-w-[117px]"
          onClick={() => navigate(`/projects/${localId}`)}
        >
          Open project
        </Button>
      ) : (
        <Button
          variant="outline"
          disabled={busy || project.latest_version == null}
          onClick={() => onPull(true)}
        >
          Pull and install
        </Button>
      )}
      <ButtonGroupSeparator />
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger
          variant="outline"
          render={<Button variant="outline" size="icon" />}
          aria-label={`More actions for ${project.name}`}
        >
          <ChevronDown aria-hidden />
        </DropdownMenuTrigger>
        <DropdownMenuPortal>
          <DropdownMenuContent align="end" className="min-w-44">
            {!localId && !isPulling && (
              <DropdownMenuItem
                disabled={busy || project.latest_version == null}
                onClick={() => onPull(false)}
              >
                Pull project
              </DropdownMenuItem>
            )}
            <DropdownMenuItem onClick={onManageAccess}>
              Manage access
            </DropdownMenuItem>
            {onDetails && (
              <DropdownMenuItem onClick={onDetails}>
                Remote details
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenuPortal>
      </DropdownMenu>
    </ButtonGroup>
  );
};
