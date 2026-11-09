// Owner: U8 (jobs). Single-choice filter button ("Status: All") with an
// optional search box, as on the prototype's Jobs page.
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuPortal,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
import { Input } from '@nebi/ui/components/ui/input';
import { ChevronDown, Search } from 'lucide-react';
import { useState } from 'react';

export const ALL = 'all';

export interface FilterOption {
  value: string;
  label: string;
}

export interface FilterMenuProps {
  /** "Status", "Job type", "Project". */
  label: string;
  /** Selected value, or ALL. */
  value: string;
  options: FilterOption[];
  onChange: (value: string) => void;
  /** Adds a search box above the options. */
  searchable?: boolean;
}

export const FilterMenu = ({
  label,
  value,
  options,
  onChange,
  searchable = false,
}: FilterMenuProps) => {
  const [query, setQuery] = useState('');
  const q = query.trim().toLowerCase();
  const shown: FilterOption[] = [
    { value: ALL, label: 'All' },
    ...options.filter((o) => o.label.toLowerCase().includes(q)),
  ];
  const current =
    value === ALL
      ? 'All'
      : (options.find((o) => o.value === value)?.label ?? 'All');

  return (
    <DropdownMenu onOpenChange={(open) => !open && setQuery('')}>
      <DropdownMenuTrigger
        variant="outline"
        aria-label={`${label}: ${current}. Change filter`}
      >
        {label}: {current}
        <ChevronDown />
      </DropdownMenuTrigger>
      <DropdownMenuPortal>
        <DropdownMenuContent className="max-h-80 w-60 overflow-y-auto">
          {searchable && (
            <div className="relative mb-1">
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={(e) => {
                  // Let the menu handle navigation keys; keep typing in the box.
                  if (
                    e.key !== 'Escape' &&
                    e.key !== 'ArrowDown' &&
                    e.key !== 'Tab'
                  ) {
                    e.stopPropagation();
                  }
                }}
                placeholder="Search"
                aria-label={`Search ${label.toLowerCase()}s`}
                className="pr-9"
              />
              <Search
                className="pointer-events-none absolute top-1/2 right-3 size-[18px] -translate-y-1/2 text-muted-foreground"
                aria-hidden
              />
            </div>
          )}
          <DropdownMenuRadioGroup
            value={value}
            onValueChange={(v: unknown) => onChange(String(v))}
          >
            {shown.map((o) => (
              <DropdownMenuRadioItem key={o.value} value={o.value}>
                <span className="truncate">{o.label}</span>
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
          {shown.length === 1 && q && (
            <p className="px-1.5 py-1 text-muted-foreground text-sm">
              No match
            </p>
          )}
        </DropdownMenuContent>
      </DropdownMenuPortal>
    </DropdownMenu>
  );
};
