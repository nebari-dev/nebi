// Owner: U8 (jobs). Notifications bell.
//
// Decision (gap 4): notifications come from local jobs only. The menu
// lists the most recent finished jobs on this machine (succeeded, failed,
// cancelled), newest first, with an unread count for jobs that finished
// since the menu was last opened; picking one opens its job page. Server
// notifications (new shares, newer versions on the server) are a
// follow-up. The "last opened" time is a per-viewer convenience kept in
// localStorage; without storage every finished job simply counts as read.
//
// The prototype has no bell (it is designed in Figma only); the layout
// follows the Nebari header recipe (ghost icon trigger, red count badge,
// 552px menu).
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuGroupLabel,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
import { headerActionClass } from '@nebi/ui/lib/header';
import { cn } from '@nebi/ui/lib/utils';
import { Bell, CircleCheck, CircleSlash, CircleX } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { formatRelative } from '@/components/jobs2/format';
import { toNotifications } from '@/components/jobs2/notifications';
import { useJobs, useNow } from '@/hooks/client/jobs';
import { useModeStore } from '@/store/modeStore';

const SEEN_KEY = 'nebi.notifications.seenAt';
/** The jobs the bell looks at (newest first). */
const RECENT_FILTER = { limit: 30 } as const;

const STATUS_ICON = {
  completed: <CircleCheck className="size-4 text-success-foreground" />,
  failed: <CircleX className="size-4 text-destructive-foreground" />,
  cancelled: <CircleSlash className="size-4 text-muted-foreground" />,
} as const;

function readSeenAt(): number {
  try {
    const raw = localStorage.getItem(SEEN_KEY);
    if (raw) return Number(raw) || 0;
    // First visit: nothing that already happened is "new".
    const now = Date.now();
    localStorage.setItem(SEEN_KEY, String(now));
    return now;
  } catch {
    return Number.POSITIVE_INFINITY;
  }
}

function writeSeenAt(at: number) {
  try {
    localStorage.setItem(SEEN_KEY, String(at));
  } catch {
    // Storage unavailable: unread state only lasts for this page.
  }
}

export const NotificationsBell = () => {
  const navigate = useNavigate();
  const now = useNow(30_000);
  const [seenAt, setSeenAt] = useState(readSeenAt);
  // The client API (and so local jobs) exists in local mode only.
  const isLocal = useModeStore((s) => s.isLocalMode());
  const { data } = useJobs(RECENT_FILTER, {
    idlePollMs: 10_000,
    enabled: isLocal,
  });
  const items = useMemo(
    () => toNotifications(data ?? [], seenAt),
    [data, seenAt],
  );
  const unread = items.filter((n) => n.unread).length;

  const markRead = () => {
    const latest = Math.max(Date.now(), ...items.map((n) => n.at));
    setSeenAt(latest);
    writeSeenAt(latest);
  };

  return (
    <DropdownMenu modal={false} onOpenChange={(open) => !open && markRead()}>
      <DropdownMenuTrigger
        variant="ghost"
        aria-label={
          unread > 0 ? `Notifications, ${unread} unread` : 'Notifications'
        }
        title="Notifications"
        className={cn('relative w-8 px-0', headerActionClass)}
      >
        <Bell />
        {unread > 0 && (
          <span
            aria-hidden
            className="absolute -top-0.5 -right-0.5 z-10 flex h-4 min-w-4 items-center justify-center rounded-full bg-[#d00000] px-1 pt-px font-semibold text-[9px] text-white tabular-nums leading-none dark:bg-[#e00000]"
          >
            {unread > 9 ? '9+' : unread}
          </span>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuPortal>
        <DropdownMenuContent
          align="end"
          className="max-h-(--available-height) w-[552px] max-w-[calc(100vw-2rem)] overflow-y-auto p-0"
        >
          <DropdownMenuGroup className="p-1">
            <DropdownMenuGroupLabel>Notifications</DropdownMenuGroupLabel>
            {items.length === 0 ? (
              <div className="flex flex-col gap-0.5 px-2 py-3 text-sm">
                <p className="font-medium text-foreground">
                  No notifications yet
                </p>
                <p className="text-muted-foreground">
                  Finished jobs on this machine show up here.
                </p>
              </div>
            ) : (
              items.map((n) => (
                <DropdownMenuItem
                  key={n.id}
                  onClick={() => navigate(`/jobs/${n.id}`)}
                  className="items-start gap-3 px-2 py-2"
                >
                  <span className="mt-0.5" aria-hidden>
                    {STATUS_ICON[n.status]}
                  </span>
                  <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                    <span className="truncate font-medium text-foreground">
                      {n.title}
                    </span>
                    <span className="truncate text-muted-foreground text-xs">
                      {n.projectName} · {formatRelative(n.at, now)}
                    </span>
                  </span>
                  {n.unread && (
                    <span
                      className="mt-1.5 size-2 shrink-0 rounded-full bg-primary"
                      role="img"
                      aria-label="Unread"
                    />
                  )}
                </DropdownMenuItem>
              ))
            )}
          </DropdownMenuGroup>
          <DropdownMenuSeparator className="my-0" />
          <div className="p-1">
            <DropdownMenuItem
              onClick={() => navigate('/jobs')}
              className="justify-center px-2 py-1.5"
            >
              View all jobs
            </DropdownMenuItem>
          </div>
        </DropdownMenuContent>
      </DropdownMenuPortal>
    </DropdownMenu>
  );
};
