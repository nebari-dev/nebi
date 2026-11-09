// Owner: U8 (jobs). Local job outcomes as notifications (gap 4: the bell
// shows local jobs only).
import type { JobNotification } from '@/types/client/jobs';
import type { JobSummary } from '@/types/contracts';
import { jobTitle } from './format';

/** How many finished jobs the notifications menu shows. */
export const MAX_NOTIFICATIONS = 8;

const STATUS_WORD = {
  completed: 'succeeded',
  failed: 'failed',
  cancelled: 'cancelled',
} as const;

/**
 * Finished jobs as notifications, newest first: "Install version 7
 * failed". A job is unread when it finished after seenAt (epoch ms).
 */
export function toNotifications(
  jobs: JobSummary[],
  seenAt: number,
): JobNotification[] {
  const out: JobNotification[] = [];
  for (const job of jobs) {
    if (
      job.status !== 'completed' &&
      job.status !== 'failed' &&
      job.status !== 'cancelled'
    ) {
      continue;
    }
    const at = Date.parse(job.completed_at ?? job.started_at ?? job.created_at);
    out.push({
      id: job.id,
      title: `${jobTitle(job)} ${STATUS_WORD[job.status]}`,
      projectName: job.project_name || job.project_id,
      status: job.status,
      at,
      unread: at > seenAt,
    });
  }
  return out.sort((a, b) => b.at - a.at).slice(0, MAX_NOTIFICATIONS);
}
