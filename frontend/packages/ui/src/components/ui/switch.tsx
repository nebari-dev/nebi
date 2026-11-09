import { Switch as SwitchPrimitive } from '@base-ui/react/switch';
import type * as React from 'react';
import { cn } from '@nebi/ui/lib/utils';

/** Switch from the Nebari design system (publish dialog registry toggles). */
function Switch({
  className,
  ...props
}: React.ComponentProps<typeof SwitchPrimitive.Root>) {
  return (
    <SwitchPrimitive.Root
      data-slot="switch"
      className={cn(
        "relative inline-flex h-[18px] w-8 shrink-0 cursor-pointer items-center rounded-full p-0.5 outline-none motion-safe:transition-colors motion-safe:duration-(--duration-fast) motion-safe:ease-(--ease-standard) before:absolute before:left-1/2 before:top-1/2 before:h-full before:min-h-6 before:w-full before:min-w-6 before:-translate-x-1/2 before:-translate-y-1/2 before:content-[''] focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background data-[checked]:bg-primary data-[checked]:hover:bg-primary-hover data-[disabled]:cursor-not-allowed data-[disabled]:opacity-50 data-[unchecked]:bg-muted-foreground/40 data-[unchecked]:hover:bg-muted-foreground/70",
        className,
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb
        data-slot="switch-thumb"
        className="pointer-events-none block size-3.5 rounded-full bg-background shadow-sm motion-safe:transition-transform motion-safe:duration-(--duration-fast) motion-safe:ease-(--ease-standard) data-[checked]:translate-x-3.5 data-[unchecked]:translate-x-0"
      />
    </SwitchPrimitive.Root>
  );
}

export { Switch };
