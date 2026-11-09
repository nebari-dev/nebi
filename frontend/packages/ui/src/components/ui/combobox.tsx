import { Combobox as ComboboxPrimitive } from '@base-ui/react/combobox';
import { CheckIcon, ChevronDownIcon, XIcon } from 'lucide-react';
import type * as React from 'react';
import { createContext, useContext } from 'react';
import { badgeVariants } from '@nebi/ui/components/ui/badge';
import { cn } from '@nebi/ui/lib/utils';

/**
 * Combobox from the Nebari design system (matches the client prototype):
 * single select with search, or multiple select with chips (platforms,
 * channels, share dialog principals, jobs project filter).
 *
 * Single:
 *   <Combobox items={items}>
 *     <ComboboxInput placeholder="Search" />
 *     <ComboboxContent>
 *       <ComboboxEmpty>No match</ComboboxEmpty>
 *       <ComboboxList>{(item) => <ComboboxItem key={item} value={item}>{item}</ComboboxItem>}</ComboboxList>
 *     </ComboboxContent>
 *   </Combobox>
 *
 * Multiple: wrap chips and the input in <ComboboxChips> and pass `multiple`.
 */
const Combobox = ComboboxPrimitive.Root;

const InChipsContext = createContext(false);

const fieldClass =
  'group/combobox relative flex min-h-8 w-full items-center gap-2 rounded-md border border-input bg-background px-3 py-1.5 text-foreground text-sm shadow-xs outline-none motion-safe:transition-[color,background-color,border-color,box-shadow] motion-safe:duration-(--duration-fast) motion-safe:ease-(--ease-standard) hover:border-border-strong has-focus-visible:border-ring has-focus-visible:ring-2 has-focus-visible:ring-ring has-focus-visible:hover:border-ring data-popup-open:border-ring data-popup-open:ring-2 data-popup-open:ring-ring data-popup-open:hover:border-ring data-disabled:cursor-not-allowed data-disabled:border-border data-disabled:bg-muted data-disabled:text-muted-foreground-strong has-disabled:cursor-not-allowed has-disabled:border-border has-disabled:bg-muted has-disabled:text-muted-foreground-strong has-aria-invalid:border-destructive-foreground has-aria-invalid:ring-2 has-aria-invalid:ring-destructive-foreground';

const inputClass =
  'min-w-0 flex-1 bg-transparent text-foreground outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:text-muted-foreground-strong disabled:placeholder:text-muted-foreground-strong';

const iconButtonClass =
  "flex size-[18px] shrink-0 cursor-pointer items-center justify-center rounded-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-[18px]";

function ComboboxTrigger({
  className,
  children,
  ...props
}: React.ComponentProps<typeof ComboboxPrimitive.Trigger>) {
  return (
    <ComboboxPrimitive.Trigger
      data-slot="combobox-trigger"
      aria-label="Open options"
      tabIndex={-1}
      className={cn(iconButtonClass, className)}
      {...props}
    >
      {children ?? (
        <ComboboxPrimitive.Icon
          data-slot="combobox-icon"
          className="flex items-center justify-center"
        >
          <ChevronDownIcon />
        </ComboboxPrimitive.Icon>
      )}
    </ComboboxPrimitive.Trigger>
  );
}

function ComboboxClear({
  className,
  children,
  ...props
}: React.ComponentProps<typeof ComboboxPrimitive.Clear>) {
  return (
    <ComboboxPrimitive.Clear
      data-slot="combobox-clear"
      aria-label="Clear selection"
      className={cn(iconButtonClass, className)}
      {...props}
    >
      {children ?? <XIcon />}
    </ComboboxPrimitive.Clear>
  );
}

function FieldEnd({ clearable }: { clearable?: boolean }) {
  return (
    <>
      {clearable && <ComboboxClear />}
      <ComboboxTrigger />
    </>
  );
}

type ComboboxInputProps = React.ComponentProps<
  typeof ComboboxPrimitive.Input
> & {
  clearable?: boolean;
  fieldClassName?: string;
};

function ComboboxInput({
  className,
  clearable,
  fieldClassName,
  ...props
}: ComboboxInputProps) {
  const inChips = useContext(InChipsContext);
  const input = (
    <ComboboxPrimitive.Input
      data-slot="combobox-input"
      className={cn(inputClass, className)}
      {...props}
    />
  );
  if (inChips) return input;
  return (
    <ComboboxPrimitive.InputGroup
      data-slot="combobox-field"
      className={cn(fieldClass, fieldClassName)}
    >
      {input}
      <FieldEnd clearable={clearable} />
    </ComboboxPrimitive.InputGroup>
  );
}

function ComboboxChips({
  className,
  clearable,
  fieldClassName,
  children,
  ...props
}: React.ComponentProps<typeof ComboboxPrimitive.Chips> & {
  clearable?: boolean;
  fieldClassName?: string;
}) {
  return (
    <InChipsContext.Provider value={true}>
      <ComboboxPrimitive.InputGroup
        data-slot="combobox-field"
        className={cn(fieldClass, 'flex-wrap', fieldClassName)}
      >
        <ComboboxPrimitive.Chips
          data-slot="combobox-chips"
          className={cn(
            'flex min-w-0 flex-1 flex-wrap items-center gap-1 *:data-[slot=combobox-input]:min-w-16',
            className,
          )}
          {...props}
        >
          {children}
        </ComboboxPrimitive.Chips>
        <FieldEnd clearable={clearable} />
      </ComboboxPrimitive.InputGroup>
    </InChipsContext.Provider>
  );
}

function ComboboxChip({
  className,
  children,
  ...props
}: React.ComponentProps<typeof ComboboxPrimitive.Chip>) {
  return (
    <ComboboxPrimitive.Chip
      data-slot="combobox-chip"
      className={cn(
        badgeVariants({ variant: 'outline' }),
        'cursor-default pr-1 outline-none focus-visible:ring-2 focus-visible:ring-ring',
        className,
      )}
      {...props}
    >
      {children}
      <ComboboxPrimitive.ChipRemove
        data-slot="combobox-chip-remove"
        aria-label={typeof children === 'string' ? `Remove ${children}` : 'Remove'}
        className="flex size-3 shrink-0 cursor-pointer items-center justify-center rounded-full text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
      >
        <XIcon />
      </ComboboxPrimitive.ChipRemove>
    </ComboboxPrimitive.Chip>
  );
}

function ComboboxContent({
  className,
  children,
  side = 'bottom',
  sideOffset = 4,
  align = 'center',
  ...props
}: React.ComponentProps<typeof ComboboxPrimitive.Popup> & {
  side?: 'top' | 'bottom' | 'left' | 'right';
  sideOffset?: number;
  align?: 'start' | 'center' | 'end';
}) {
  return (
    <ComboboxPrimitive.Portal>
      <ComboboxPrimitive.Positioner
        side={side}
        sideOffset={sideOffset}
        align={align}
        className="isolate z-50"
      >
        <ComboboxPrimitive.Popup
          data-slot="combobox-content"
          className={cn(
            'relative isolate z-50 max-h-(--available-height) w-(--anchor-width) min-w-36 origin-(--transform-origin) overflow-x-hidden overflow-y-auto rounded-lg bg-popover p-1 text-popover-foreground opacity-100 shadow-md ring-1 ring-foreground/10 data-[starting-style]:translate-y-1 data-[starting-style]:opacity-0 data-[ending-style]:translate-y-1 data-[ending-style]:opacity-0 motion-safe:transition-[opacity,transform] motion-safe:duration-(--duration-base) motion-safe:ease-(--ease-emphasized)',
            className,
          )}
          {...props}
        >
          {children}
        </ComboboxPrimitive.Popup>
      </ComboboxPrimitive.Positioner>
    </ComboboxPrimitive.Portal>
  );
}

function ComboboxList({
  className,
  ...props
}: React.ComponentProps<typeof ComboboxPrimitive.List>) {
  return (
    <ComboboxPrimitive.List
      data-slot="combobox-list"
      className={cn('scroll-py-1 outline-none', className)}
      {...props}
    />
  );
}

function ComboboxItem({
  className,
  children,
  ...props
}: React.ComponentProps<typeof ComboboxPrimitive.Item>) {
  return (
    <ComboboxPrimitive.Item
      data-slot="combobox-item"
      className={cn(
        "relative flex w-full cursor-default items-center gap-2 rounded-md px-1.5 py-1 text-sm outline-hidden select-none not-data-[disabled]:data-[highlighted]:bg-muted not-data-[disabled]:data-[highlighted]:text-foreground data-[disabled]:cursor-not-allowed data-[disabled]:text-muted-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        className,
      )}
      {...props}
    >
      <span className="flex min-w-0 flex-1 truncate">{children}</span>
      <ComboboxPrimitive.ItemIndicator
        data-slot="combobox-item-indicator"
        className="pointer-events-none flex size-4 shrink-0 items-center justify-center"
      >
        <CheckIcon />
      </ComboboxPrimitive.ItemIndicator>
    </ComboboxPrimitive.Item>
  );
}

function ComboboxEmpty({
  className,
  children = 'No results found.',
  ...props
}: React.ComponentProps<typeof ComboboxPrimitive.Empty>) {
  return (
    <ComboboxPrimitive.Empty
      data-slot="combobox-empty"
      className={cn(
        'px-3 py-5 text-center text-muted-foreground-strong text-sm empty:p-0',
        className,
      )}
      {...props}
    >
      {children}
    </ComboboxPrimitive.Empty>
  );
}

export {
  Combobox,
  ComboboxChip,
  ComboboxChips,
  ComboboxClear,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxTrigger,
};
