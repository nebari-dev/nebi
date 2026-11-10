// Owner: U2 (detail). Sync marker of a version row: Newest here, Newest on
// server or In sync (VersionItem.sync, filled by the remote SDK).
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@nebi/ui/components/ui/tooltip';
import { cn } from '@nebi/ui/lib/utils';
import { Laptop, Server } from 'lucide-react';
import { SYNC_MARKER_LABELS, type SyncMarker } from '@/types/contracts';
import { SYNC_MARKER_TIPS } from './format';

export const SyncMarkerLabel = ({
  marker,
  compact = false,
}: {
  marker: SyncMarker;
  /** Icon only (label for screen readers), when the row is crowded. */
  compact?: boolean;
}) => {
  const label = SYNC_MARKER_LABELS[marker];
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            data-sync={marker}
            className={cn(
              'flex items-center gap-1 text-xs leading-4',
              marker === 'in_sync'
                ? 'text-success-foreground'
                : 'text-muted-foreground-strong',
            )}
          />
        }
      >
        {marker === 'newest_here' && (
          <Laptop className="size-3.5 shrink-0" aria-hidden />
        )}
        {marker === 'newest_on_server' && (
          <Server className="size-3.5 shrink-0" aria-hidden />
        )}
        {marker === 'in_sync' && (
          <span className="flex items-center" aria-hidden>
            <Laptop data-sync-part="laptop" className="size-3.5 shrink-0" />
            <Server data-sync-part="server" className="size-3.5 shrink-0" />
          </span>
        )}
        {compact ? (
          <span className="sr-only">{label}</span>
        ) : (
          <span data-sync-part="label">{label}</span>
        )}
      </TooltipTrigger>
      <TooltipContent>{SYNC_MARKER_TIPS[marker]}</TooltipContent>
    </Tooltip>
  );
};
