import { cva, type VariantProps } from 'class-variance-authority';
import type * as React from 'react';
import { cn } from '@nebi/ui/lib/utils';

const buttonGroupVariants = cva(
  'm-0 flex min-w-0 w-fit items-stretch border-0 p-0 *:focus-visible:relative *:focus-visible:z-10 has-[>[data-slot=button-group]]:gap-2 [&>input]:flex-1',
  {
    variants: {
      orientation: {
        horizontal:
          'flex-row [&>[data-slot]]:rounded-r-none [&>[data-slot]~[data-slot]]:-ml-px [&>[data-slot]~[data-slot]]:rounded-l-none [&>[data-slot]:not(:has(~[data-slot]))]:rounded-r-md',
        vertical:
          'flex-col [&>[data-slot]]:rounded-b-none [&>[data-slot]~[data-slot]]:-mt-px [&>[data-slot]~[data-slot]]:rounded-t-none [&>[data-slot]:not(:has(~[data-slot]))]:rounded-b-md',
      },
    },
    defaultVariants: { orientation: 'horizontal' },
  },
);

/**
 * Joins adjacent buttons (e.g. the "New project" split button: primary
 * action + menu trigger).
 */
function ButtonGroup({
  className,
  orientation = 'horizontal',
  ...props
}: React.ComponentProps<'fieldset'> &
  VariantProps<typeof buttonGroupVariants>) {
  return (
    <fieldset
      data-slot="button-group"
      data-orientation={orientation}
      className={cn(buttonGroupVariants({ orientation }), className)}
      {...props}
    />
  );
}

function ButtonGroupSeparator({
  className,
  orientation = 'vertical',
  ...props
}: React.ComponentProps<'hr'> & { orientation?: 'horizontal' | 'vertical' }) {
  return (
    <hr
      aria-orientation={orientation}
      data-slot="button-group-separator"
      data-orientation={orientation}
      className={cn(
        'relative shrink-0 self-stretch border-0 bg-border',
        orientation === 'vertical' ? 'my-px w-px' : 'mx-px h-px',
        className,
      )}
      {...props}
    />
  );
}

export { ButtonGroup, ButtonGroupSeparator, buttonGroupVariants };
