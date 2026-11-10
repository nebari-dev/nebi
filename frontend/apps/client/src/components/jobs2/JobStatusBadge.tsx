// Owner: U8 (jobs). Job status chip: Queued (outline), Running (info),
// Succeeded (success), Failed (destructive), Cancelled (muted).
import { Badge } from '@nebi/ui/components/ui/badge';
import { cn } from '@nebi/ui/lib/utils';
import { JOB_STATUS_LABELS, type JobStatus } from '@/types/contracts';

const VARIANT = {
  pending: 'outline',
  running: 'info',
  completed: 'success',
  failed: 'destructive',
  cancelled: 'default',
} as const;

export const JobStatusBadge = ({
  status,
  className,
}: {
  status: JobStatus;
  className?: string;
}) => (
  <Badge
    variant={VARIANT[status]}
    className={cn(
      status === 'cancelled' &&
        'border-transparent bg-muted text-muted-foreground',
      className,
    )}
  >
    {JOB_STATUS_LABELS[status]}
  </Badge>
);
