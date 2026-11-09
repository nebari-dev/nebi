// Owner: U5 (remote). The version rail sync marker (prototype: "Newest
// here", "Newest on server", "In sync") with its tooltip. U2 may render it
// in the version rows: <SyncMarkerLabel marker={version.sync} />.
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@nebi/ui/components/ui/tooltip';
import { cn } from '@nebi/ui/lib/utils';
import { Laptop, Server } from 'lucide-react';
import { SYNC_MARKER_LABELS, type SyncMarker } from '@/types/contracts';
import { SYNC_MARKER_TIPS } from './copy';

export interface SyncMarkerLabelProps {
  marker: SyncMarker | null | undefined;
  /** Icon only (label kept for screen readers). */
  compact?: boolean;
}

export const SyncMarkerLabel = ({ marker, compact }: SyncMarkerLabelProps) => {
  if (!marker) return null;
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
            <Laptop className="size-3.5 shrink-0" />
            <Server className="size-3.5 shrink-0" />
          </span>
        )}
        {compact ? (
          <span className="sr-only">{label}</span>
        ) : (
          <span>{label}</span>
        )}
      </TooltipTrigger>
      <TooltipContent>{SYNC_MARKER_TIPS[marker]}</TooltipContent>
    </Tooltip>
  );
};
