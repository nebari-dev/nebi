import { Menu as MenuPrimitive } from '@base-ui/react/menu';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuPortal,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
import { useTheme } from '@nebi/ui/hooks/theme-provider';
import { isThemeMode } from '@nebi/ui/hooks/use-theme-preference';
import { headerActionClass } from '@nebi/ui/lib/header';
import { cn } from '@nebi/ui/lib/utils';
import { Monitor, Moon, Sun } from 'lucide-react';

const OPTIONS = [
  { value: 'light', label: 'Light', Icon: Sun },
  { value: 'dark', label: 'Dark', Icon: Moon },
  { value: 'system', label: 'System', Icon: Monitor },
] as const;

/**
 * Header theme menu (Light / Dark / System), matching the client prototype:
 * an icon button that opens a segmented radio group. Must be rendered inside
 * a ThemeProvider.
 */
function ThemeMenu({ className }: { className?: string }) {
  const { themeMode, isDarkMode, setThemeMode } = useTheme();
  const TriggerIcon =
    themeMode === 'system' ? Monitor : isDarkMode ? Moon : Sun;
  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        variant="ghost"
        aria-label="Theme"
        className={cn('w-8 px-0', headerActionClass, className)}
      >
        <TriggerIcon />
      </DropdownMenuTrigger>
      <DropdownMenuPortal>
        <DropdownMenuContent align="end" className="w-[248px] min-w-0 p-2">
          <p className="px-1.5 pb-2 font-medium text-foreground text-sm">
            Theme
          </p>
          <MenuPrimitive.RadioGroup
            aria-label="Theme"
            value={themeMode}
            onValueChange={(value: unknown) => {
              if (isThemeMode(value)) setThemeMode(value);
            }}
            className="flex h-[34px] items-center gap-1 rounded-md bg-muted p-1"
          >
            {OPTIONS.map(({ value, label, Icon }) => (
              <MenuPrimitive.RadioItem
                key={value}
                value={value}
                aria-label={`${label} mode`}
                title={`${label} mode`}
                closeOnClick={false}
                className={cn(
                  'flex h-auto flex-1 cursor-pointer items-center justify-center gap-1 rounded-sm border border-transparent px-1.5 py-0.5 font-medium text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring',
                  'text-muted-foreground-strong hover:text-foreground',
                  'data-checked:border-border-strong data-checked:bg-card data-checked:text-foreground data-checked:shadow-[0_1px_3px_0_rgba(0,0,0,0.10)]',
                )}
              >
                <Icon className="size-4" aria-hidden />
                <span>{label}</span>
              </MenuPrimitive.RadioItem>
            ))}
          </MenuPrimitive.RadioGroup>
        </DropdownMenuContent>
      </DropdownMenuPortal>
    </DropdownMenu>
  );
}

export { ThemeMenu };
