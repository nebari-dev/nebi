import * as React from 'react';
import { cn } from '@/lib/utils';

// Trimmed to the parts this app uses (like card.tsx): no AlertTitle and no
// destructive variant. Re-add from shadcn if a call site needs them.
const Alert = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    role="alert"
    className={cn(
      'relative grid w-full grid-cols-[0_1fr] items-start gap-y-0.5 rounded-lg border bg-background px-4 py-3 text-sm text-foreground has-[>svg]:grid-cols-[calc(var(--spacing)*4)_1fr] has-[>svg]:gap-x-3 [&>svg]:size-4 [&>svg]:translate-y-0.5 [&>svg]:text-current',
      className,
    )}
    {...props}
  />
));
Alert.displayName = 'Alert';

const AlertDescription = React.forwardRef<
  HTMLParagraphElement,
  React.HTMLAttributes<HTMLParagraphElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn('col-start-2 text-sm [&_p]:leading-relaxed', className)}
    {...props}
  />
));
AlertDescription.displayName = 'AlertDescription';

export { Alert, AlertDescription };
