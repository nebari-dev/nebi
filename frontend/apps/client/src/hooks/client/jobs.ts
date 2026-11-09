// Owner: U8 (jobs). Jobs queries, cancel, and the reusable job progress
// toast every unit uses after submitting a job.
import { toast } from '@nebi/ui/components/ui/toast';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback } from 'react';
import { jobsApi } from '@/api/client/jobs';
import { queryClient } from '@/lib/queryClient';
import { queryKeys } from '@/lib/queryKeys';
import {
  isActiveJob,
  type JobListFilter,
  type JobSummary,
} from '@/types/contracts';

export const useJobs = (filter: JobListFilter = {}) =>
  useQuery({
    queryKey: queryKeys.jobList(filter),
    queryFn: () => jobsApi.list(filter),
    refetchInterval: (query) =>
      query.state.data?.some(isActiveJob) ? 2000 : false,
  });

export const useJob = (id: string) =>
  useQuery({
    queryKey: queryKeys.job(id),
    queryFn: () => jobsApi.get(id),
    enabled: !!id,
    refetchInterval: (query) =>
      query.state.data && isActiveJob(query.state.data) ? 2000 : false,
  });

export const useCancelJob = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => jobsApi.cancel(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.jobs }),
  });
};

export interface JobToastOptions {
  /** Title while running, e.g. "Installing version 5". */
  title?: string;
  /** Title when the job succeeds, e.g. "Version 8 created". */
  successTitle?: string | ((job: JobSummary) => string);
  /** Description when the job succeeds, e.g. "Ready to use on this machine." */
  successDescription?: string;
  /** Title when the job fails. */
  errorTitle?: string;
  /** Called once the job reaches a final state. */
  onSettled?: (job: JobSummary) => void;
  /** Called when the user picks "View job" (navigate to /jobs/:id). */
  onViewJob?: (job: JobSummary) => void;
  /** Show a "Cancel" action while the job runs (cancellable toast). */
  cancellable?: boolean;
  /** Poll interval in ms (default 1000). */
  intervalMs?: number;
}

/**
 * Shows a progress toast for a submitted job and follows it to the end.
 * Usable outside React. Invalidates the projects and jobs queries when the
 * job settles. Returns a function that stops following the job.
 */
export function trackJobToast(job: JobSummary, opts: JobToastOptions = {}) {
  const title = opts.title ?? job.type_label;
  const id = toast.add({
    type: 'loading',
    title,
    timeout: 0,
    actionProps: opts.cancellable
      ? { children: 'Cancel', onClick: () => void jobsApi.cancel(job.id) }
      : undefined,
  });
  let stopped = false;
  const settle = (final: JobSummary) => {
    stopped = true;
    queryClient.invalidateQueries({ queryKey: queryKeys.projects });
    queryClient.invalidateQueries({ queryKey: queryKeys.jobs });
    const viewJob = opts.onViewJob
      ? { children: 'View job', onClick: () => opts.onViewJob?.(final) }
      : undefined;
    if (final.status === 'completed') {
      const success =
        typeof opts.successTitle === 'function'
          ? opts.successTitle(final)
          : (opts.successTitle ?? `${title} finished`);
      toast.update(id, {
        type: 'success',
        title: success,
        description: opts.successDescription,
        timeout: 5000,
        actionProps: undefined,
      });
    } else if (final.status === 'cancelled') {
      toast.update(id, {
        type: 'info',
        title: `${title} cancelled`,
        timeout: 5000,
        actionProps: undefined,
      });
    } else {
      toast.update(id, {
        type: 'error',
        title: opts.errorTitle ?? `${title} failed`,
        description: final.error,
        timeout: viewJob ? 0 : 8000,
        actionProps: viewJob,
      });
    }
    opts.onSettled?.(final);
  };
  if (!isActiveJob(job)) {
    settle(job);
    return () => {};
  }
  const timer = setInterval(async () => {
    if (stopped) return;
    try {
      const current = await jobsApi.get(job.id);
      if (!isActiveJob(current)) {
        clearInterval(timer);
        settle(current);
      }
    } catch {
      // Keep polling; the local API is loopback and recovers quickly.
    }
  }, opts.intervalMs ?? 1000);
  return () => {
    stopped = true;
    clearInterval(timer);
  };
}

/** React wrapper around trackJobToast. */
export const useJobToast = () =>
  useCallback(
    (job: JobSummary, opts?: JobToastOptions) => trackJobToast(job, opts),
    [],
  );
