import { Select as SelectPrimitive } from '@base-ui-components/react/select';
import { CheckIcon, ChevronDownIcon, ChevronUpIcon } from 'lucide-react';
import type * as React from 'react';
import { cn } from '@/lib/utils';

// Local compatibility exception: the published @nebari/select Trigger.Props
// omits React's ref prop, but PublishDialog preserves pre-migration autofocus
// on the registry combobox. Remove this when the registry exposes trigger refs.
type SelectTriggerProps = React.ComponentProps<typeof SelectPrimitive.Trigger>;

type SelectContentProps = SelectPrimitive.Popup.Props &
  Pick<
    SelectPrimitive.Positioner.Props,
    'align' | 'alignOffset' | 'side' | 'sideOffset' | 'alignItemWithTrigger'
  >;

/**
 * Select groups the trigger, value, popup, and option items. Value, open state,
 * disabled/read-only behavior, and Field integration come from Base UI.
 */
const Select = SelectPrimitive.Root;

const selectTriggerClassName =
  "flex h-9 w-full items-center justify-between gap-1.5 rounded-md border border-input bg-background py-2 pr-2 pl-2.5 text-sm whitespace-nowrap shadow-xs outline-none hover:border-border-strong focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring focus-visible:hover:border-ring data-[pressed]:border-ring data-[pressed]:ring-2 data-[pressed]:ring-ring data-[pressed]:hover:border-ring data-[popup-open]:border-ring data-[popup-open]:ring-2 data-[popup-open]:ring-ring data-[popup-open]:hover:border-ring disabled:cursor-not-allowed disabled:border-border disabled:bg-muted disabled:text-muted-foreground aria-invalid:border-destructive-foreground aria-invalid:ring-2 aria-invalid:ring-destructive-foreground aria-invalid:hover:border-destructive-foreground aria-invalid:focus-visible:border-destructive-foreground aria-invalid:focus-visible:ring-destructive-foreground aria-invalid:data-[pressed]:border-destructive-foreground aria-invalid:data-[pressed]:ring-destructive-foreground aria-invalid:data-[popup-open]:border-destructive-foreground aria-invalid:data-[popup-open]:ring-destructive-foreground data-placeholder:text-muted-foreground *:data-[slot=select-value]:line-clamp-1 *:data-[slot=select-value]:flex *:data-[slot=select-value]:items-center *:data-[slot=select-value]:gap-1.5 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4";

/** Groups related options inside the popup list. */
function SelectGroup({ className, ...props }: SelectPrimitive.Group.Props) {
  return (
    <SelectPrimitive.Group
      data-slot="select-group"
      className={cn('scroll-my-1', className)}
      {...props}
    />
  );
}

/** Displays the selected value inside the trigger. */
function SelectValue({ className, ...props }: SelectPrimitive.Value.Props) {
  return (
    <SelectPrimitive.Value
      data-slot="select-value"
      className={cn('flex flex-1 text-left', className)}
      {...props}
    />
  );
}

/** Combobox button that opens the popup and shows the selected value. */
function SelectTrigger({
  className,
  children,
  ref,
  ...props
}: SelectTriggerProps) {
  return (
    <SelectPrimitive.Trigger
      data-slot="select-trigger"
      ref={ref}
      className={cn(selectTriggerClassName, className)}
      {...props}
    >
      {children}
      <SelectPrimitive.Icon
        render={
          <ChevronDownIcon className="pointer-events-none size-4 text-muted-foreground" />
        }
      />
    </SelectPrimitive.Trigger>
  );
}

/**
 * Portaled popup that positions the option list against the trigger and adds
 * scroll affordances for long menus.
 */
function SelectContent({
  className,
  children,
  side = 'bottom',
  sideOffset = 0,
  align = 'center',
  alignOffset = 0,
  alignItemWithTrigger = false,
  ...props
}: SelectContentProps) {
  return (
    <SelectPrimitive.Portal>
      <SelectPrimitive.Positioner
        side={side}
        sideOffset={sideOffset}
        align={align}
        alignOffset={alignOffset}
        alignItemWithTrigger={alignItemWithTrigger}
        className="isolate z-50"
      >
        <SelectPrimitive.Popup
          data-slot="select-content"
          data-align-trigger={alignItemWithTrigger}
          className={cn(
            'relative isolate z-50 max-h-(--available-height) w-(--anchor-width) min-w-36 origin-(--transform-origin) overflow-x-hidden overflow-y-auto rounded-md bg-popover text-popover-foreground opacity-100 shadow-md ring-1 ring-foreground/10 data-[starting-style]:translate-y-1 data-[starting-style]:opacity-0 data-[ending-style]:translate-y-1 data-[ending-style]:opacity-0 motion-safe:transition-[opacity,transform] motion-safe:duration-[--duration-base] motion-safe:ease-[--ease-emphasized]',
            className,
          )}
          {...props}
        >
          <SelectScrollUpButton />
          <SelectPrimitive.List className="p-1">
            {children}
          </SelectPrimitive.List>
          <SelectScrollDownButton />
        </SelectPrimitive.Popup>
      </SelectPrimitive.Positioner>
    </SelectPrimitive.Portal>
  );
}

/** Label for a `SelectGroup`, rendered inside the popup list. */
function SelectLabel({
  className,
  ...props
}: SelectPrimitive.GroupLabel.Props) {
  return (
    <SelectPrimitive.GroupLabel
      data-slot="select-label"
      className={cn(
        'px-2 py-1.5 text-xs uppercase text-muted-foreground',
        className,
      )}
      {...props}
    />
  );
}

/** Selectable option row. Disabled items are announced and cannot be selected. */
function SelectItem({
  className,
  children,
  ...props
}: SelectPrimitive.Item.Props) {
  return (
    <SelectPrimitive.Item
      data-slot="select-item"
      className={cn(
        "relative flex w-full cursor-default items-center gap-2 rounded-sm py-1.5 pr-8 pl-2 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground not-data-[variant=destructive]:focus:**:text-accent-foreground data-[disabled]:cursor-not-allowed data-[disabled]:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 *:[span]:last:flex *:[span]:last:items-center *:[span]:last:gap-2",
        className,
      )}
      {...props}
    >
      <SelectPrimitive.ItemText className="flex flex-1 shrink-0 gap-2 whitespace-nowrap">
        {children}
      </SelectPrimitive.ItemText>
      <SelectPrimitive.ItemIndicator
        render={
          <span className="pointer-events-none absolute right-2 flex size-4 items-center justify-center" />
        }
      >
        <CheckIcon className="pointer-events-none" />
      </SelectPrimitive.ItemIndicator>
    </SelectPrimitive.Item>
  );
}

/** Visual separator between option groups inside the popup list. */
function SelectSeparator({
  className,
  ...props
}: SelectPrimitive.Separator.Props) {
  return (
    <SelectPrimitive.Separator
      data-slot="select-separator"
      className={cn('pointer-events-none -mx-1 my-1 h-px bg-border', className)}
      {...props}
    />
  );
}

/**
 * Scroll affordance shown when options overflow above the visible popup area.
 */
function SelectScrollUpButton({
  className,
  ...props
}: React.ComponentProps<typeof SelectPrimitive.ScrollUpArrow>) {
  return (
    <SelectPrimitive.ScrollUpArrow
      data-slot="select-scroll-up-button"
      className={cn(
        "top-0 z-10 flex w-full cursor-default items-center justify-center bg-popover py-1 [&_svg:not([class*='size-'])]:size-4",
        className,
      )}
      {...props}
    >
      <ChevronUpIcon />
    </SelectPrimitive.ScrollUpArrow>
  );
}

/**
 * Scroll affordance shown when options overflow below the visible popup area.
 */
function SelectScrollDownButton({
  className,
  ...props
}: React.ComponentProps<typeof SelectPrimitive.ScrollDownArrow>) {
  return (
    <SelectPrimitive.ScrollDownArrow
      data-slot="select-scroll-down-button"
      className={cn(
        "bottom-0 z-10 flex w-full cursor-default items-center justify-center bg-popover py-1 [&_svg:not([class*='size-'])]:size-4",
        className,
      )}
      {...props}
    >
      <ChevronDownIcon />
    </SelectPrimitive.ScrollDownArrow>
  );
}

export {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectScrollDownButton,
  SelectScrollUpButton,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
};
