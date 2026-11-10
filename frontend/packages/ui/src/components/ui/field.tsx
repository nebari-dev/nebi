import { Field as FieldPrimitive } from '@base-ui/react/field';
import type * as React from 'react';
import { cn } from '@nebi/ui/lib/utils';

/**
 * Form field from the Nebari design system: label, control, description and
 * error wired for accessibility by Base UI.
 */
function Field({
  className,
  ...props
}: React.ComponentProps<typeof FieldPrimitive.Root>) {
  return (
    <FieldPrimitive.Root
      data-slot="field"
      className={cn('flex flex-col gap-1.5', className)}
      {...props}
    />
  );
}

function FieldLabel({
  className,
  ...props
}: React.ComponentProps<typeof FieldPrimitive.Label>) {
  return (
    <FieldPrimitive.Label
      data-slot="field-label"
      className={cn(
        'font-medium text-sm data-[disabled]:cursor-not-allowed data-[disabled]:opacity-50',
        className,
      )}
      {...props}
    />
  );
}

function FieldDescription({
  className,
  ...props
}: React.ComponentProps<typeof FieldPrimitive.Description>) {
  return (
    <FieldPrimitive.Description
      data-slot="field-description"
      className={cn('text-muted-foreground text-sm', className)}
      {...props}
    />
  );
}

function FieldError({
  className,
  ...props
}: React.ComponentProps<typeof FieldPrimitive.Error>) {
  return (
    <FieldPrimitive.Error
      data-slot="field-error"
      className={cn('text-destructive-foreground text-sm', className)}
      {...props}
    />
  );
}

function FieldControl({
  className,
  ...props
}: React.ComponentProps<typeof FieldPrimitive.Control>) {
  return (
    <FieldPrimitive.Control
      data-slot="input"
      className={cn(
        'flex w-full rounded-md border border-input bg-background px-3 py-1.5 text-foreground text-sm outline-none placeholder:text-muted-foreground motion-safe:transition-[color,background-color,border-color,box-shadow] motion-safe:duration-(--duration-fast) motion-safe:ease-(--ease-standard) hover:border-border-strong focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:bg-muted disabled:opacity-50 aria-invalid:border-destructive-foreground aria-invalid:ring-2 aria-invalid:ring-destructive-foreground data-[invalid]:border-destructive-foreground data-[invalid]:ring-2 data-[invalid]:ring-destructive-foreground',
        className,
      )}
      {...props}
    />
  );
}

export { Field, FieldControl, FieldDescription, FieldError, FieldLabel };
