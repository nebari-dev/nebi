// Owner: U8 (jobs). Notifications bell. Decision (gap 4): notifications
// come from local jobs only; server-side notifications are a follow-up.

import { Button } from '@nebi/ui/components/ui/button';
import { headerActionClass } from '@nebi/ui/lib/header';
import { cn } from '@nebi/ui/lib/utils';
import { Bell } from 'lucide-react';
import { Link } from 'react-router-dom';

export const NotificationsBell = () => (
  <Button
    variant="ghost"
    size="icon"
    aria-label="Notifications"
    title="Notifications"
    className={cn('w-8 px-0', headerActionClass)}
    render={<Link to="/jobs" />}
  >
    <Bell />
  </Button>
);
