import { Tooltip as TooltipPrimitive } from '@base-ui/react/tooltip';
import type * as React from 'react';
import { cn } from '@nebi/ui/lib/utils';

/**
 * Tooltip from the Nebari design system (matches the client prototype).
 * Wrap the app (or a subtree) in a TooltipProvider so tooltips share delay.
 *
 * <Tooltip>
 *   <TooltipTrigger render={<button type="button" />}>…</TooltipTrigger>
 *   <TooltipContent>Last installed version</TooltipContent>
 * </Tooltip>
 */
function TooltipProvider({
  delay = 0,
  closeDelay = 100,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Provider>) {
  return (
    <TooltipPrimitive.Provider
      data-slot="tooltip-provider"
      delay={delay}
      closeDelay={closeDelay}
      {...props}
    />
  );
}

function Tooltip(props: React.ComponentProps<typeof TooltipPrimitive.Root>) {
  return <TooltipPrimitive.Root data-slot="tooltip" {...props} />;
}

function TooltipTrigger(
  props: React.ComponentProps<typeof TooltipPrimitive.Trigger>,
) {
  return <TooltipPrimitive.Trigger data-slot="tooltip-trigger" {...props} />;
}

type TooltipContentProps = React.ComponentProps<
  typeof TooltipPrimitive.Popup
> & {
  side?: 'top' | 'bottom' | 'left' | 'right';
  sideOffset?: number;
  align?: 'start' | 'center' | 'end';
  showArrow?: boolean;
};

function TooltipContent({
  className,
  side = 'top',
  sideOffset = 8,
  align = 'center',
  showArrow = true,
  children,
  ...props
}: TooltipContentProps) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Positioner
        side={side}
        sideOffset={sideOffset}
        align={align}
        className="isolate z-50"
      >
        <TooltipPrimitive.Popup
          data-slot="tooltip-content"
          className={cn(
            'relative z-50 w-max max-w-xs origin-(--transform-origin) rounded-sm bg-foreground px-2.5 py-1.5 text-background text-xs leading-4 shadow-md outline-none data-[starting-style]:scale-95 data-[starting-style]:opacity-0 data-[ending-style]:scale-95 data-[ending-style]:opacity-0 motion-safe:transition-[opacity,transform] motion-safe:duration-(--duration-fast) motion-safe:ease-(--ease-emphasized)',
            className,
          )}
          {...props}
        >
          {children}
          {showArrow && (
            <TooltipPrimitive.Arrow
              data-slot="tooltip-arrow"
              className="pointer-events-none z-50 size-2.5 rotate-45 bg-foreground data-[side=bottom]:bottom-full data-[side=bottom]:translate-y-1/2 data-[side=left]:left-full data-[side=left]:-translate-x-1/2 data-[side=right]:right-full data-[side=right]:translate-x-1/2 data-[side=top]:top-full data-[side=top]:-translate-y-1/2"
            />
          )}
        </TooltipPrimitive.Popup>
      </TooltipPrimitive.Positioner>
    </TooltipPrimitive.Portal>
  );
}

export type { TooltipContentProps };
export { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger };
