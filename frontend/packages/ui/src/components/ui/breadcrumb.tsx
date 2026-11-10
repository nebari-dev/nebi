import { useRender } from '@base-ui/react/use-render';
import { ChevronRight } from 'lucide-react';
import type * as React from 'react';
import { cn } from '@nebi/ui/lib/utils';

/** Breadcrumb from the Nebari design system (Projects > name). */
function Breadcrumb({ className, ...props }: React.ComponentProps<'nav'>) {
  return (
    <nav
      aria-label="breadcrumb"
      data-slot="breadcrumb"
      className={cn('min-w-0', className)}
      {...props}
    />
  );
}

function BreadcrumbList({ className, ...props }: React.ComponentProps<'ol'>) {
  return (
    <ol
      data-slot="breadcrumb-list"
      className={cn(
        'flex min-w-0 flex-nowrap items-center gap-1.5 whitespace-nowrap text-muted-foreground text-sm leading-5 sm:gap-2.5',
        className,
      )}
      {...props}
    />
  );
}

function BreadcrumbItem({ className, ...props }: React.ComponentProps<'li'>) {
  return (
    <li
      data-slot="breadcrumb-item"
      className={cn('inline-flex items-center gap-1.5', className)}
      {...props}
    />
  );
}

function BreadcrumbLink({
  className,
  ref,
  render,
  ...props
}: useRender.ComponentProps<'a'>) {
  return useRender({
    defaultTagName: 'a',
    render,
    ref,
    props: {
      'data-slot': 'breadcrumb-link',
      className: cn(
        'rounded-[4px] outline-none underline-offset-4 hover:text-foreground hover:underline focus-visible:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background motion-safe:transition-[color,box-shadow] motion-safe:duration-(--duration-fast) motion-safe:ease-(--ease-standard)',
        className,
      ),
      ...props,
    },
  });
}

function BreadcrumbPage({ className, ...props }: React.ComponentProps<'span'>) {
  return (
    <span
      aria-current="page"
      aria-disabled="true"
      data-slot="breadcrumb-page"
      className={cn('font-normal text-foreground', className)}
      {...props}
    />
  );
}

function BreadcrumbSeparator({
  className,
  ...props
}: React.ComponentProps<'li'>) {
  return (
    <li
      aria-hidden="true"
      data-slot="breadcrumb-separator"
      role="presentation"
      className={cn(
        'flex items-center text-muted-foreground [&>svg]:size-4 [&>svg]:shrink-0',
        className,
      )}
      {...props}
    >
      <ChevronRight aria-hidden="true" />
    </li>
  );
}

export {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
};
