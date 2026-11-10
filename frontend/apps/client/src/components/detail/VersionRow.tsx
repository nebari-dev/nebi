// Owner: U2 (detail). One row of the version rail (prototype VersionRow):
// vN, Installing/Installed, first tag (+N), sync marker, Published badge,
// author avatar and description.
import { PersonAvatar } from '@nebi/ui/components/ui/avatar';
import { Badge } from '@nebi/ui/components/ui/badge';
import { Spinner } from '@nebi/ui/components/ui/spinner';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@nebi/ui/components/ui/tooltip';
import { cn } from '@nebi/ui/lib/utils';
import { Check, PackageCheck } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { useJustCreated } from '@/components/newversion/justCreated';
import type { VersionItem } from '@/types/contracts';
import { publishedReferences } from './format';
import { SyncMarkerLabel } from './SyncMarker';

const versionRowClass = cn(
  'flex w-full flex-col gap-1 rounded-md border border-transparent bg-background px-3 py-2 text-left outline-none',
  'hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring',
  'motion-safe:transition-[background-color,border-color] motion-safe:duration-(--duration-fast) motion-safe:ease-(--ease-standard)',
);

/** How long a just-created row stays lit, then how long it fades. */
const LIT_MS = 600;
const FADE_MS = 1200;

export interface VersionRowProps {
  version: VersionItem;
  /** The version just appeared (e.g. created): the row lights up briefly. */
  justCreated?: boolean;
  selected: boolean;
  installing: boolean;
  onSelect: () => void;
}

export const VersionRow = ({
  version,
  justCreated = false,
  selected,
  installing,
  onSelect,
}: VersionRowProps) => {
  const [glow, setGlow] = useState<'lit' | 'fading' | null>(() =>
    justCreated ? 'lit' : null,
  );
  useEffect(() => {
    if (!glow) return;
    const t = window.setTimeout(
      () => setGlow(glow === 'lit' ? 'fading' : null),
      glow === 'lit' ? LIT_MS : FADE_MS,
    );
    return () => window.clearTimeout(t);
  }, [glow]);
  const [firstTag, ...moreTags] = version.tags;
  const crowded =
    (version.installed || installing) &&
    firstTag != null &&
    moreTags.length > 0;
  const refs = publishedReferences(version);
  const { id } = useParams();
  // "Version N created": the new row lights up, then fades (U3).
  const created = useJustCreated(
    version.server_only ? undefined : id,
    version.number,
  );
  // A row that appears while the rail is mounted (glow) or that the new
  // version page just created before navigating here (created) lights up.
  const lit = glow ?? created;
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-current={selected ? 'true' : undefined}
      data-just-created={lit ? '' : undefined}
      className={cn(
        versionRowClass,
        selected && 'border-border-strong bg-muted',
        lit === 'lit' && 'border-primary/40 bg-primary/10 hover:bg-primary/10',
        lit === 'fading' &&
          'motion-safe:duration-[1200ms] motion-safe:ease-out',
      )}
    >
      <span className="flex min-h-5 w-full items-center gap-1.5">
        <span className="font-medium text-foreground text-sm leading-5">
          v{version.number}
        </span>
        {installing ? (
          <Badge className="border-transparent bg-transparent px-1 text-info-foreground">
            <Spinner className="size-3" />
            Installing
          </Badge>
        ) : (
          version.installed && (
            <Badge variant="success">
              <Check />
              Installed
            </Badge>
          )
        )}
        {firstTag && (
          <Badge variant="outline" className="max-w-24">
            <span className="truncate">{firstTag}</span>
          </Badge>
        )}
        {moreTags.length > 0 && (
          <Tooltip>
            <TooltipTrigger render={<Badge variant="outline" />}>
              +{moreTags.length}
            </TooltipTrigger>
            <TooltipContent>{moreTags.join(', ')}</TooltipContent>
          </Tooltip>
        )}
        <span className="ml-auto flex items-center gap-2">
          {version.sync && (
            <SyncMarkerLabel marker={version.sync} compact={crowded} />
          )}
          {refs.length > 0 && (
            <Tooltip>
              <TooltipTrigger render={<Badge variant="secondary" />}>
                <PackageCheck />
                Published
              </TooltipTrigger>
              <TooltipContent>Published to {refs.join(', ')}</TooltipContent>
            </Tooltip>
          )}
        </span>
      </span>
      <span className="flex min-w-0 items-center gap-1.5">
        <PersonAvatar
          name={version.author.name}
          focusable={false}
          className="size-5 text-[10px]"
        />
        <span className="truncate text-muted-foreground-strong text-xs leading-4">
          {version.description}
        </span>
      </span>
    </button>
  );
};
