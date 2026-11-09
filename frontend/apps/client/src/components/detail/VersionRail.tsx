// Owner: U2 (detail). The version rail: search, filters, versions grouped
// by age, and a collapsed mode showing only version numbers.
import { Button } from '@nebi/ui/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuGroupLabel,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
import { Input } from '@nebi/ui/components/ui/input';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@nebi/ui/components/ui/tooltip';
import { cn } from '@nebi/ui/lib/utils';
import { ChevronDown, ChevronsLeft, ChevronsRight, Search } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import type { VersionItem } from '@/types/contracts';
import { groupVersionsByAge } from './format';
import { VersionRow } from './VersionRow';
import {
  availableFilters,
  filterVersions,
  VERSION_FILTER_LABELS,
  type VersionFilter,
  type VersionFilterContext,
} from './versionFilters';

const railClass = cn(
  'flex min-h-0 shrink-0 flex-col gap-3 overflow-hidden',
  'motion-safe:transition-[width] motion-safe:duration-200 motion-safe:ease-out',
);

export interface VersionRailProps {
  versions: VersionItem[];
  selected: number | undefined;
  installingVersion: number | null;
  filterContext: VersionFilterContext;
  onSelect: (n: number) => void;
}

export const VersionRail = ({
  versions,
  selected,
  installingVersion,
  filterContext,
  onSelect,
}: VersionRailProps) => {
  const [query, setQuery] = useState('');
  const [filters, setFilters] = useState<VersionFilter[]>([]);
  const [collapsed, setCollapsed] = useState(false);
  const listRef = useRef<HTMLElement>(null);
  const toggleRef = useRef<HTMLButtonElement>(null);
  const toggled = useRef(false);

  const options = useMemo(
    () => availableFilters(versions, filterContext),
    [versions, filterContext],
  );
  const activeFilters = filters.filter((f) => options.includes(f));
  const groups = useMemo(
    () =>
      groupVersionsByAge(
        filterVersions(versions, query, activeFilters, filterContext),
      ),
    [versions, query, activeFilters, filterContext],
  );

  // Keep the selected version in view and the toggle focused after toggling.
  // biome-ignore lint/correctness/useExhaustiveDependencies: runs when the rail is shown or hidden
  useEffect(() => {
    listRef.current
      ?.querySelector('[aria-current="true"]')
      ?.scrollIntoView?.({ block: 'nearest' });
    if (toggled.current) toggleRef.current?.focus();
  }, [collapsed]);

  const toggleLabel = collapsed ? 'Show versions' : 'Hide versions';
  const toggle = (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            ref={toggleRef}
            variant="outline"
            size="icon-sm"
            aria-label={toggleLabel}
            aria-expanded={!collapsed}
            onClick={() => {
              toggled.current = true;
              setCollapsed(!collapsed);
            }}
          />
        }
      >
        {collapsed ? <ChevronsRight /> : <ChevronsLeft />}
      </TooltipTrigger>
      <TooltipContent side={collapsed ? 'right' : 'top'}>
        {toggleLabel}
      </TooltipContent>
    </Tooltip>
  );

  if (collapsed) {
    const all = groupVersionsByAge(versions);
    return (
      <aside
        className={cn(railClass, 'w-8')}
        aria-label="Versions"
        data-tour="rail"
      >
        <div className="flex min-h-0 w-8 flex-1 flex-col items-center gap-3">
          {toggle}
          <nav
            ref={listRef}
            aria-label="Version history"
            className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto pb-9"
          >
            {all.map((g) => (
              <ul
                key={g.label}
                aria-label={g.label}
                className="flex flex-col gap-1"
              >
                {g.versions.map((v) => {
                  const label = `Version ${v.number} · ${v.description}`;
                  const isSelected = v.number === selected;
                  return (
                    <li key={`${v.number}-${v.server_only}`}>
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <button
                              type="button"
                              aria-label={label}
                              aria-current={isSelected ? 'true' : undefined}
                              onClick={() => onSelect(v.number)}
                              className={cn(
                                'flex size-8 items-center justify-center rounded-md border border-transparent bg-background font-medium text-foreground text-sm leading-5 outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset',
                                isSelected && 'border-border-strong bg-muted',
                              )}
                            />
                          }
                        >
                          v{v.number}
                        </TooltipTrigger>
                        <TooltipContent side="right">{label}</TooltipContent>
                      </Tooltip>
                    </li>
                  );
                })}
              </ul>
            ))}
          </nav>
        </div>
      </aside>
    );
  }

  return (
    <aside
      className={cn(railClass, 'w-[374px] max-w-[45%]')}
      aria-label="Versions"
      data-tour="rail"
    >
      <div className="flex flex-col">
        <div className="flex items-center justify-between">
          <h2 className="font-semibold text-base text-foreground leading-5">
            Versions
          </h2>
          {toggle}
        </div>
        <div className="mt-3 flex items-center gap-2">
          <div className="relative flex-1">
            <Search
              className="pointer-events-none absolute top-1/2 left-3 z-10 size-[18px] -translate-y-1/2 text-muted-foreground"
              aria-hidden
            />
            <Input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search versions"
              aria-label="Search versions"
              className="pl-9"
            />
          </div>
          <DropdownMenu modal={false}>
            <DropdownMenuTrigger variant="outline" className="gap-1.5">
              Filters
              {activeFilters.length > 0 && (
                <span className="rounded-full bg-primary px-1.5 text-primary-foreground text-xs leading-4">
                  {activeFilters.length}
                  <span className="sr-only"> active</span>
                </span>
              )}
              <ChevronDown />
            </DropdownMenuTrigger>
            <DropdownMenuPortal>
              <DropdownMenuContent align="end" className="w-60">
                <DropdownMenuGroup>
                  <DropdownMenuGroupLabel>Show only</DropdownMenuGroupLabel>
                  {options.map((f) => (
                    <DropdownMenuCheckboxItem
                      key={f}
                      checked={activeFilters.includes(f)}
                      closeOnClick={false}
                      onCheckedChange={(checked) =>
                        setFilters((prev) =>
                          checked
                            ? [...prev.filter((p) => p !== f), f]
                            : prev.filter((p) => p !== f),
                        )
                      }
                    >
                      {VERSION_FILTER_LABELS[f]}
                    </DropdownMenuCheckboxItem>
                  ))}
                </DropdownMenuGroup>
                {activeFilters.length > 0 && (
                  <>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem onClick={() => setFilters([])}>
                      Clear filters
                    </DropdownMenuItem>
                  </>
                )}
              </DropdownMenuContent>
            </DropdownMenuPortal>
          </DropdownMenu>
        </div>
      </div>
      <nav
        ref={listRef}
        aria-label="Version history"
        className="-mx-1 flex min-h-0 flex-1 flex-col overflow-y-auto px-1 pb-9"
      >
        {groups.map((g) => (
          <section key={g.label} className="flex flex-col">
            <h3 className="mt-4 mb-1.5 font-medium text-muted-foreground text-xs leading-4 first:mt-1">
              {g.label}
            </h3>
            <ul className="flex flex-col gap-1.5">
              {g.versions.map((v) => (
                <li key={`${v.number}-${v.server_only}`}>
                  <VersionRow
                    version={v}
                    selected={v.number === selected}
                    installing={
                      !v.server_only && v.number === installingVersion
                    }
                    onSelect={() => onSelect(v.number)}
                  />
                </li>
              ))}
            </ul>
          </section>
        ))}
        {groups.length === 0 && (
          <p className="px-3 py-6 text-muted-foreground text-sm">
            {query.trim()
              ? `No versions match “${query}”.`
              : 'No versions match these filters.'}
          </p>
        )}
      </nav>
    </aside>
  );
};
