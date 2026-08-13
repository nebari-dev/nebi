import { ChevronDown, LogOut, Monitor, Moon, Sun } from 'lucide-react';
import { type ReactNode, useId, useRef, useState } from 'react';
import type { ThemeMode } from '@/hooks/useThemePreference';
import { cn } from '@/lib/utils';
import type { User } from '@/types';

type ProfileMenuProps = {
  user: User | null;
  themeMode: ThemeMode;
  onThemeChange: (themeMode: ThemeMode) => void;
  onLogout: () => void;
};

const getInitial = (user: User | null) =>
  (user?.username || user?.email || 'U').charAt(0).toUpperCase();

export const ProfileMenu = ({
  user,
  themeMode,
  onThemeChange,
  onLogout,
}: ProfileMenuProps) => {
  const [open, setOpen] = useState(false);
  const [failedAvatarUrl, setFailedAvatarUrl] = useState<string | null>(null);
  const menuId = useId();
  const triggerRef = useRef<HTMLButtonElement>(null);

  const displayName = user?.username || user?.email || 'User';
  const avatarUrl = user?.avatar_url;
  const avatarError = Boolean(avatarUrl && failedAvatarUrl === avatarUrl);

  const handleLogout = () => {
    setOpen(false);
    onLogout();
  };

  const handleAvatarError = () => {
    if (avatarUrl) {
      setFailedAvatarUrl(avatarUrl);
    }
  };

  const handleAvatarLoad = () => {
    setFailedAvatarUrl(null);
  };

  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: This container observes bubbled React focus and keyboard events from its child controls.
    <div
      className="relative"
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) {
          setOpen(false);
        }
      }}
      onKeyDown={(event) => {
        if (event.key === 'Escape') {
          setOpen(false);
          triggerRef.current?.focus();
        }
      }}
    >
      <button
        ref={triggerRef}
        type="button"
        className="flex h-10 max-w-56 items-center gap-2 rounded-md px-2 text-sm font-medium text-foreground transition-colors hover:bg-nav-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        onClick={() => setOpen((value) => !value)}
      >
        {avatarUrl && !avatarError ? (
          <img
            src={avatarUrl}
            alt=""
            className="h-8 w-8 shrink-0 rounded-full object-cover"
            referrerPolicy="no-referrer-when-downgrade"
            crossOrigin="anonymous"
            onError={handleAvatarError}
            onLoad={handleAvatarLoad}
          />
        ) : (
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-sm font-medium text-primary">
            {getInitial(user)}
          </span>
        )}
        <span className="min-w-0 truncate">{displayName}</span>
        <ChevronDown
          className={cn(
            'h-4 w-4 shrink-0 text-muted-foreground transition-transform',
            open && 'rotate-180',
          )}
        />
      </button>
      {open && (
        <div
          id={menuId}
          role="menu"
          aria-label="Profile menu"
          className="absolute right-0 top-full z-50 mt-2 w-72 max-w-[calc(100vw-2rem)] rounded-md border bg-popover p-2 text-popover-foreground shadow-lg"
        >
          <div
            role="presentation"
            className="flex items-center gap-3 px-2 py-2"
          >
            {avatarUrl && !avatarError ? (
              <img
                src={avatarUrl}
                alt=""
                className="h-10 w-10 shrink-0 rounded-full object-cover"
                referrerPolicy="no-referrer-when-downgrade"
                crossOrigin="anonymous"
                onError={handleAvatarError}
                onLoad={handleAvatarLoad}
              />
            ) : (
              <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-base font-medium text-primary">
                {getInitial(user)}
              </span>
            )}
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold text-foreground">
                {displayName}
              </p>
              {user?.email && (
                <p className="truncate text-sm text-muted-foreground">
                  {user.email}
                </p>
              )}
            </div>
          </div>
          <hr className="my-1 border-border" />
          <div className="px-2 py-2">
            <fieldset
              aria-label="Theme"
              className="flex min-w-0 items-center gap-1 rounded-lg border-0 bg-muted p-1"
            >
              <ThemeOption
                label="Light mode"
                text="Light"
                selected={themeMode === 'light'}
                onSelect={() => onThemeChange('light')}
              >
                <Sun className="h-4 w-4" />
              </ThemeOption>
              <ThemeOption
                label="Dark mode"
                text="Dark"
                selected={themeMode === 'dark'}
                onSelect={() => onThemeChange('dark')}
              >
                <Moon className="h-4 w-4" />
              </ThemeOption>
              <ThemeOption
                label="System theme"
                text="System"
                selected={themeMode === 'system'}
                onSelect={() => onThemeChange('system')}
              >
                <Monitor className="h-4 w-4" />
              </ThemeOption>
            </fieldset>
          </div>
          <hr className="my-1 border-border" />
          <button
            type="button"
            role="menuitem"
            className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-sm font-medium text-red-500 transition-colors hover:bg-red-500/10 focus:bg-red-500/10 focus:outline-none"
            onClick={handleLogout}
          >
            <LogOut className="h-4 w-4" />
            Sign out
          </button>
        </div>
      )}
    </div>
  );
};

type ThemeOptionProps = {
  label: string;
  text: string;
  selected: boolean;
  onSelect: () => void;
  children: ReactNode;
};

const ThemeOption = ({
  label,
  text,
  selected,
  onSelect,
  children,
}: ThemeOptionProps) => (
  <button
    type="button"
    role="menuitemradio"
    aria-label={label}
    aria-checked={selected}
    title={label}
    className={cn(
      'flex flex-1 items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
      selected
        ? 'bg-background text-foreground shadow-sm'
        : 'text-foreground hover:text-foreground',
    )}
    onClick={onSelect}
  >
    {children}
    <span>{text}</span>
  </button>
);
