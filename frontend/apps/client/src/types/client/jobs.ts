// Owner: the jobs unit. Unit-specific frontend types; the API contract types
// live in types/contracts.ts (frozen).
import type { JobStatus, JobType } from '@/types/contracts';

/** Jobs page filters ("all" = no filter). Kept in the URL query. */
export interface JobsPageFilters {
  status: JobStatus | 'all';
  type: JobType | 'all';
  project: string | 'all';
  /** Started column: newest first (default) or oldest first. */
  newestFirst: boolean;
}

/** A finished local job shown in the notifications menu. */
export interface JobNotification {
  id: string;
  title: string;
  projectName: string;
  status: Extract<JobStatus, 'completed' | 'failed' | 'cancelled'>;
  /** Epoch ms when the job finished. */
  at: number;
  unread: boolean;
}
