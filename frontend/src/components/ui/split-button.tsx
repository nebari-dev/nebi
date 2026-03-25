import { useState, useRef, useEffect } from 'react';
import { ChevronDown } from 'lucide-react';
import { Button } from '@/components/ui/button';

export type SplitButtonMenuItem = {
  label: string;
  icon?: React.ReactNode;
  onClick: () => void;
};

export const SplitButton = ({
  onPrimary,
  primaryLabel,
  menuItems,
}: {
  onPrimary: () => void;
  primaryLabel: React.ReactNode;
  menuItems: SplitButtonMenuItem[];
}) => {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  return (
    <div ref={ref} className="relative flex">
      <Button
        className="rounded-r-none border-r border-primary-foreground/20"
        onClick={onPrimary}
      >
        {primaryLabel}
      </Button>
      <Button
        className="rounded-l-none px-2"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="menu"
        aria-expanded={open}
      >
        <ChevronDown className="h-4 w-4" />
      </Button>
      {open && (
        <div className="absolute right-0 top-full mt-1 z-50 min-w-[256px] rounded-md border bg-popover text-popover-foreground shadow-md overflow-hidden">
          {menuItems.map((item) => (
            <button
              key={item.label}
              className="flex w-full items-center gap-2 px-3 py-2 text-sm hover:bg-accent hover:text-accent-foreground transition-colors"
              onClick={() => {
                setOpen(false);
                item.onClick();
              }}
            >
              {item.icon}
              {item.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
};
