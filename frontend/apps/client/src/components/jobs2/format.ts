// Owner: U8 (jobs). Time formatting for the Jobs pages and the
// notifications menu, matching the prototype ("Just now", "5 min ago",
// "2m 05s", "9 Nov, 16:50").
import type { JobSummary } from '@/types/contracts';

const MONTHS = [
  'Jan',
  'Feb',
  'Mar',
  'Apr',
  'May',
  'Jun',
  'Jul',
  'Aug',
  'Sep',
  'Oct',
  'Nov',
  'Dec',
];

/** Placeholder for a time or duration that does not exist yet. */
export const EMPTY_VALUE = '-';

/** 42s, 2m 05s, 1h 03m. */
export function formatDuration(ms: number): string {
  const total = Math.max(0, Math.round(ms / 1000));
  if (total < 60) return `${total}s`;
  const minutes = Math.floor(total / 60);
  if (minutes < 60) {
    return `${minutes}m ${String(total % 60).padStart(2, '0')}s`;
  }
  return `${Math.floor(minutes / 60)}h ${String(minutes % 60).padStart(2, '0')}m`;
}

/**
 * How long the job ran: start to end, or start to now while it runs.
 * Null when the job never started.
 */
export function jobDuration(job: JobSummary, now: number): string | null {
  if (!job.started_at) return null;
  const start = Date.parse(job.started_at);
  const end = job.completed_at ? Date.parse(job.completed_at) : now;
  return formatDuration(end - start);
}

/** "Just now", "5 min ago", "2 hours ago", "Yesterday", "3 Nov". */
export function formatRelative(at: number, now: number): string {
  const seconds = (now - at) / 1000;
  if (seconds < 60) return 'Just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.floor(minutes / 60);
  const today = new Date(now);
  today.setHours(0, 0, 0, 0);
  if (at >= today.getTime()) {
    return hours === 1 ? '1 hour ago' : `${hours} hours ago`;
  }
  if (at >= today.getTime() - 24 * 60 * 60 * 1000) return 'Yesterday';
  const d = new Date(at);
  return `${d.getDate()} ${MONTHS[d.getMonth()]}`;
}

/** "9 Nov, 16:50". */
export function formatDateTime(iso: string): string {
  const d = new Date(iso);
  return `${d.getDate()} ${MONTHS[d.getMonth()]}, ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

/** When the job started, or was queued if it has not started. */
export const jobStartedAt = (job: JobSummary): number =>
  Date.parse(job.started_at ?? job.created_at);

/** The first 8 characters of a job id (log file names, CLI). */
export const shortJobId = (id: string) => id.slice(0, 8);

/**
 * Job title: the type label, numbered for the jobs that act on one version
 * ("Install version 7", "Create version 8"). Other jobs keep their label;
 * their version shows on the job page.
 */
export function jobTitle(
  job: Pick<JobSummary, 'type' | 'type_label' | 'version_number'>,
): string {
  if (
    job.version_number != null &&
    (job.type === 'install_version' || job.type === 'create_version')
  ) {
    return `${job.type_label} ${job.version_number}`;
  }
  return job.type_label;
}

/** Download name for a job log: nebi-<type>-<id8>.log. */
export const logFileName = (job: Pick<JobSummary, 'type' | 'id'>) =>
  `nebi-${job.type}-${shortJobId(job.id)}.log`;
