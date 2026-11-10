import { Toast as ToastPrimitive } from '@base-ui/react/toast';
import { cva } from 'class-variance-authority';
import {
  CircleAlert,
  CircleCheck,
  Info,
  LoaderCircle,
  TriangleAlert,
  X,
} from 'lucide-react';
import type * as React from 'react';
import { useMemo } from 'react';
import { Button } from '@nebi/ui/components/ui/button';
import { cn } from '@nebi/ui/lib/utils';

/**
 * Toasts from the Nebari design system, matching the client prototype:
 * bottom-right stack, optional icon by type, optional action button (e.g.
 * "Undo", "View job", "Cancel") and a dismiss button.
 *
 * Mount <Toaster /> once near the app root (inside providers), then call
 * `toast.add(...)` from anywhere or `useToast()` inside React.
 *
 *   toast.add({ type: 'success', title: 'Access removed',
 *     actionProps: { children: 'Undo', onClick: undo } });
 *
 * Toasts with an action stay until dismissed (timeout 0), like the
 * prototype; warnings and errors are announced with high priority.
 */

type ToastType = 'default' | 'success' | 'warning' | 'error' | 'info' | 'loading';

type ToastData = {
  /** Hide the leading icon (default: shown). */
  showIcon?: boolean;
  /** Hide the dismiss button when there is no action (default: shown). */
  dismissible?: boolean;
};

type ToastOptions = ToastPrimitive.Root.ToastObject<ToastData> extends infer T
  ? Omit<Partial<T & { type: ToastType }>, 'id'> & { id?: string }
  : never;

function normalizeType(type?: string): ToastType {
  switch (type) {
    case 'success':
    case 'warning':
    case 'error':
    case 'info':
    case 'loading':
      return type;
    default:
      return 'default';
  }
}

const HIGH_PRIORITY = new Set<ToastType>(['warning', 'error']);

// biome-ignore lint/suspicious/noExplicitAny: options mirror Base UI's loose typing
function withDefaults<T extends Record<string, any>>(options: T): T {
  const extra: Record<string, unknown> = {};
  if (options.priority === undefined && options.type !== undefined) {
    extra.priority = HIGH_PRIORITY.has(normalizeType(options.type))
      ? 'high'
      : 'low';
  }
  if (options.actionProps != null && options.timeout === undefined) {
    extra.timeout = 0;
  }
  return Object.keys(extra).length > 0 ? { ...options, ...extra } : options;
}

const baseManager = ToastPrimitive.createToastManager<ToastData>();

/** Global toast API (usable outside React, e.g. in mutation callbacks). */
const toast = {
  add: (options: ToastOptions) => baseManager.add(withDefaults(options)),
  update: (id: string, options: Omit<ToastOptions, 'id'>) =>
    baseManager.update(id, withDefaults(options)),
  close: (id?: string) => baseManager.close(id),
  success: (title: React.ReactNode, options?: Omit<ToastOptions, 'title'>) =>
    baseManager.add(withDefaults({ ...options, title, type: 'success' })),
  error: (title: React.ReactNode, options?: Omit<ToastOptions, 'title'>) =>
    baseManager.add(withDefaults({ ...options, title, type: 'error' })),
  info: (title: React.ReactNode, options?: Omit<ToastOptions, 'title'>) =>
    baseManager.add(withDefaults({ ...options, title, type: 'info' })),
  loading: (title: React.ReactNode, options?: Omit<ToastOptions, 'title'>) =>
    baseManager.add(withDefaults({ ...options, title, type: 'loading' })),
};

/** Toast API bound to the nearest Toaster (same as `toast`). */
function useToast() {
  const manager = ToastPrimitive.useToastManager<ToastData>();
  return useMemo(
    () => ({
      ...toast,
      toasts: manager.toasts,
    }),
    [manager.toasts],
  );
}

const toastIconVariants = cva(
  'flex shrink-0 items-start overflow-hidden pt-px [&_svg]:pointer-events-none [&_svg]:size-[18px]',
  {
    variants: {
      variant: {
        default: 'text-muted-foreground',
        success: 'text-success-foreground',
        warning: 'text-warning-foreground',
        error: 'text-destructive-foreground',
        info: 'text-info-foreground',
        loading: 'text-muted-foreground',
      },
    },
    defaultVariants: { variant: 'default' },
  },
);

const toastRootClass = cn(
  'group/toast pointer-events-auto absolute right-0 bottom-0 z-[calc(1000-var(--toast-index))] h-(--height) w-full origin-bottom rounded-md border border-border bg-popover text-popover-foreground shadow-lg will-change-transform outline-none select-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
  '[--gap:0.75rem] [--height:var(--toast-frontmost-height,var(--toast-height))] [--offset-y:calc(var(--toast-offset-y)*-1+calc(var(--toast-index)*var(--gap)*-1)+var(--toast-swipe-movement-y))] [--peek:0.75rem] [--scale:calc(max(0,1-(var(--toast-index)*0.1)))] [--shrink:calc(1-var(--scale))]',
  '[transform:translateX(var(--toast-swipe-movement-x))_translateY(calc(var(--toast-swipe-movement-y)-(var(--toast-index)*var(--peek))-(var(--shrink)*var(--toast-height))))_scale(var(--scale))]',
  "after:absolute after:top-full after:left-0 after:h-[calc(var(--gap)+1px)] after:w-full after:content-['']",
  'data-expanded:h-(--toast-height) data-expanded:[transform:translateX(var(--toast-swipe-movement-x))_translateY(var(--offset-y))]',
  'data-limited:opacity-0 data-starting-style:translate-y-[150%]',
  '[&[data-ending-style]:not([data-limited]):not([data-swipe-direction])]:translate-y-[150%]',
  'data-ending-style:data-[swipe-direction=down]:[transform:translateY(calc(var(--toast-swipe-movement-y)+150%))]',
  'data-ending-style:data-[swipe-direction=left]:[transform:translateX(calc(var(--toast-swipe-movement-x)-150%))_translateY(var(--offset-y))]',
  'data-ending-style:data-[swipe-direction=right]:[transform:translateX(calc(var(--toast-swipe-movement-x)+150%))_translateY(var(--offset-y))]',
  'data-ending-style:data-[swipe-direction=up]:[transform:translateY(calc(var(--toast-swipe-movement-y)-150%))]',
  'data-swiping:motion-safe:duration-0 motion-safe:transition-[opacity,transform] motion-safe:duration-(--duration-slow) motion-safe:ease-(--ease-emphasized)',
);

function ToastIcon({ type }: { type?: string }) {
  const variant = normalizeType(type);
  let icon: React.ReactNode;
  switch (variant) {
    case 'success':
      icon = <CircleCheck aria-hidden="true" />;
      break;
    case 'warning':
      icon = <TriangleAlert aria-hidden="true" />;
      break;
    case 'error':
      icon = <CircleAlert aria-hidden="true" />;
      break;
    case 'info':
      icon = <Info aria-hidden="true" />;
      break;
    case 'loading':
      icon = (
        <LoaderCircle aria-hidden="true" className="motion-safe:animate-spin" />
      );
      break;
    default:
      icon = <Info aria-hidden="true" />;
  }
  return (
    <span
      data-slot="toast-icon"
      data-variant={variant}
      className={toastIconVariants({ variant })}
    >
      {icon}
    </span>
  );
}

function ToastList() {
  const { toasts } = ToastPrimitive.useToastManager<ToastData>();
  return toasts.map((t) => {
    const variant = normalizeType(t.type);
    const showIcon = t.data?.showIcon !== false;
    const showClose = t.actionProps != null || t.data?.dismissible !== false;
    return (
      <ToastPrimitive.Root
        key={t.id}
        toast={t}
        data-slot="toast"
        data-variant={variant}
        aria-busy={variant === 'loading' || undefined}
        className={toastRootClass}
      >
        <ToastPrimitive.Content
          data-slot="toast-content"
          className="flex h-full items-center gap-3 overflow-hidden p-3 opacity-100 has-data-[slot=toast-description]:items-start data-behind:opacity-0 data-expanded:opacity-100 motion-safe:transition-opacity motion-safe:duration-(--duration-fast) motion-safe:ease-(--ease-standard)"
        >
          {showIcon && <ToastIcon type={t.type} />}
          <div className="flex min-w-0 flex-1 flex-col gap-1 overflow-hidden break-words">
            <ToastPrimitive.Title
              data-slot="toast-title"
              className="font-medium text-sm leading-5"
            />
            <ToastPrimitive.Description
              data-slot="toast-description"
              className="text-muted-foreground text-sm leading-5"
            />
          </div>
          <ToastPrimitive.Action
            data-slot="toast-action"
            render={<Button variant="outline" size="sm" />}
            className="shrink-0 shadow-none focus-visible:ring-offset-popover"
          />
          {showClose && (
            <ToastPrimitive.Close
              data-slot="toast-close"
              aria-label="Dismiss"
              render={<Button variant="ghost" size="icon-sm" />}
              className="relative shrink-0 text-foreground after:absolute after:-inset-2 after:content-[''] focus-visible:ring-offset-popover"
            >
              <X aria-hidden="true" className="size-4" />
            </ToastPrimitive.Close>
          )}
        </ToastPrimitive.Content>
      </ToastPrimitive.Root>
    );
  });
}

/** Mount once: provider, portal, viewport and the toast stack. */
function Toaster({
  children,
  limit = 5,
  className,
}: {
  children?: React.ReactNode;
  limit?: number;
  className?: string;
}) {
  return (
    <ToastPrimitive.Provider toastManager={baseManager} limit={limit}>
      {children}
      <ToastPrimitive.Portal data-slot="toast-portal">
        <ToastPrimitive.Viewport
          data-slot="toast-viewport"
          className={cn(
            // z-[60] keeps toasts (and their Undo actions) above an open
            // dialog's z-50 viewport, which would otherwise swallow clicks.
            'pointer-events-none fixed inset-x-4 bottom-4 z-[60] mx-auto w-auto max-w-[400px] outline-none sm:right-4 sm:left-auto sm:mx-0 sm:w-full',
            className,
          )}
        >
          <ToastList />
        </ToastPrimitive.Viewport>
      </ToastPrimitive.Portal>
    </ToastPrimitive.Provider>
  );
}

export type { ToastData, ToastOptions, ToastType };
export { Toaster, toast, useToast };
