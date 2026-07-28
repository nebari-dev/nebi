import { Tabs as TabsPrimitive } from '@base-ui-components/react/tabs';
import { cva, type VariantProps } from 'class-variance-authority';
import { type ComponentProps, createContext, useContext } from 'react';
import { cn } from '@/lib/utils';

const tabsListVariants = cva(
  'group/tabs-list relative isolate inline-flex w-fit items-center text-sm font-medium text-muted-foreground data-[orientation=vertical]:flex-col data-[orientation=vertical]:items-stretch',
  {
    variants: {
      variant: {
        pill: 'h-9 gap-1 rounded-md bg-background p-[3px] data-[orientation=vertical]:h-auto',
        underline: 'gap-6 border-b border-border',
        // Compatibility aliases for the initial API while the component is new.
        line: 'gap-6 border-b border-border',
        toggle:
          'h-9 gap-1 rounded-md bg-background p-[3px] data-[orientation=vertical]:h-auto',
      },
    },
    defaultVariants: {
      variant: 'pill',
    },
  },
);

const tabsTabVariants = cva(
  'relative z-10 inline-flex min-w-0 items-center justify-center gap-1.5 whitespace-nowrap px-3 py-2 text-sm font-medium underline-offset-4 outline-none hover:text-foreground hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background data-[active]:text-foreground data-[disabled]:pointer-events-none data-[disabled]:text-muted-foreground/50 data-[disabled]:no-underline data-[orientation=vertical]:justify-start disabled:pointer-events-none disabled:text-muted-foreground/50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*="size-"])]:size-4',
  {
    variants: {
      variant: {
        pill: 'h-[calc(100%-1px)] rounded-sm border border-border bg-card px-2 py-1 data-[active]:border-border-strong data-[active]:shadow-[0_1px_1.5px_rgb(0_0_0_/_0.1)] data-[disabled]:bg-muted data-[orientation=vertical]:h-8',
        underline: 'rounded-none px-0 pb-2',
        line: 'rounded-none px-0 pb-2',
        toggle:
          'h-[calc(100%-1px)] rounded-sm border border-border bg-card px-2 py-1 data-[active]:border-border-strong data-[active]:shadow-[0_1px_1.5px_rgb(0_0_0_/_0.1)] data-[disabled]:bg-muted data-[orientation=vertical]:h-8',
      },
    },
    defaultVariants: {
      variant: 'pill',
    },
  },
);

const tabsIndicatorVariants = cva(
  'pointer-events-none absolute left-[var(--active-tab-left)] top-[var(--active-tab-top)] z-0 h-[var(--active-tab-height)] w-[var(--active-tab-width)]',
  {
    variants: {
      variant: {
        pill: 'hidden',
        underline: 'top-auto bottom-0 h-0.5 rounded-full bg-primary',
        line: 'top-auto bottom-0 h-0.5 rounded-full bg-primary',
        toggle: 'hidden',
      },
    },
    defaultVariants: {
      variant: 'pill',
    },
  },
);

type TabsVariant = NonNullable<
  VariantProps<typeof tabsListVariants>['variant']
>;

const DEFAULT_TABS_VARIANT: TabsVariant = 'pill';
const TabsVariantContext = createContext<TabsVariant>(DEFAULT_TABS_VARIANT);

type TabsProps = ComponentProps<typeof TabsPrimitive.Root>;

type TabsListProps = ComponentProps<typeof TabsPrimitive.List> &
  VariantProps<typeof tabsListVariants>;

type TabsTabProps = ComponentProps<typeof TabsPrimitive.Tab> &
  VariantProps<typeof tabsTabVariants>;

type TabsIndicatorProps = ComponentProps<typeof TabsPrimitive.Indicator> &
  VariantProps<typeof tabsIndicatorVariants>;

type TabsPanelProps = ComponentProps<typeof TabsPrimitive.Panel>;

/**
 * Tabs implemented on top of Base UI's `Tabs`. Base UI owns roving focus,
 * Arrow/Home/End keyboard behavior, and `aria-selected`/panel wiring.
 */
function Tabs({ className, ...props }: TabsProps) {
  return (
    <TabsPrimitive.Root
      data-slot="tabs"
      className={cn(
        'flex flex-col gap-4 data-[orientation=vertical]:flex-row',
        className,
      )}
      {...props}
    />
  );
}

/**
 * Groups the tab buttons and controls the visual variant. `pill` renders
 * individual bordered tabs; `underline` renders header-style tabs with a
 * primary underline indicator.
 */
function TabsList({ className, variant, ...props }: TabsListProps) {
  const selectedVariant = variant ?? DEFAULT_TABS_VARIANT;

  return (
    <TabsVariantContext.Provider value={selectedVariant}>
      <TabsPrimitive.List
        data-slot="tabs-list"
        data-variant={selectedVariant}
        className={cn(
          tabsListVariants({ variant: selectedVariant }),
          className,
        )}
        {...props}
      />
    </TabsVariantContext.Provider>
  );
}

/**
 * Individual tab button. By default it inherits the variant from `TabsList`;
 * pass `variant` only when a one-off override is needed.
 */
function TabsTab({ className, variant, ...props }: TabsTabProps) {
  const contextVariant = useContext(TabsVariantContext);
  const selectedVariant = variant ?? contextVariant;

  return (
    <TabsPrimitive.Tab
      data-slot="tabs-tab"
      data-variant={selectedVariant}
      className={cn(tabsTabVariants({ variant: selectedVariant }), className)}
      {...props}
    />
  );
}

/**
 * Active tab marker. Place it inside `TabsList` after the `TabsTab` children.
 */
function TabsIndicator({ className, variant, ...props }: TabsIndicatorProps) {
  const contextVariant = useContext(TabsVariantContext);
  const selectedVariant = variant ?? contextVariant;

  return (
    <TabsPrimitive.Indicator
      data-slot="tabs-indicator"
      data-variant={selectedVariant}
      className={cn(
        tabsIndicatorVariants({ variant: selectedVariant }),
        className,
      )}
      {...props}
    />
  );
}

function TabsPanel({ className, ...props }: TabsPanelProps) {
  return (
    <TabsPrimitive.Panel
      data-slot="tabs-panel"
      className={cn(
        'text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
        className,
      )}
      {...props}
    />
  );
}

export type {
  TabsIndicatorProps,
  TabsListProps,
  TabsPanelProps,
  TabsProps,
  TabsTabProps,
};
export {
  Tabs,
  TabsIndicator,
  TabsList,
  TabsPanel,
  TabsTab,
  tabsIndicatorVariants,
  tabsListVariants,
  tabsTabVariants,
};
