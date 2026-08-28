import type { JobStatus, JobType } from '@/types';

export const NEBI_JOB_COMPLETED_MESSAGE = 'nebi:job-completed';

export type NebiJobCompletedMessage = {
  type: typeof NEBI_JOB_COMPLETED_MESSAGE;
  jobType: JobType;
  status: Extract<JobStatus, 'completed'>;
  workspaceId: string;
};

export const isEmbedded = () =>
  typeof window !== 'undefined' && window.parent !== window;

export function postToHost(message: NebiJobCompletedMessage): void {
  if (!isEmbedded()) {
    return;
  }

  // Nebi's Jupyter server-proxy iframe is same-origin; cross-origin framing is
  // intentionally not covered.
  window.parent.postMessage(message, window.location.origin);
}
