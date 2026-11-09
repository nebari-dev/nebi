// Owner: U8 (jobs). Jobs queries, cancel, the live log stream, and the
// reusable job progress toast every unit uses after submitting a job.
import { toast } from '@nebi/ui/components/ui/toast';
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { useCallback, useEffect, useState } from 'react';
import { clientApiError, httpStatus } from '@/api/client/base';
import { jobsApi } from '@/api/client/jobs';
import { queryClient } from '@/lib/queryClient';
import { queryKeys } from '@/lib/queryKeys';
import {
  isActiveJob,
  type JobListFilter,
  type JobSummary,
} from '@/types/contracts';

export interface UseJobsOptions {
  /**
   * Poll interval while no listed job is active. Jobs can start from the
   * CLI or another window, so lists that must notice new jobs (Jobs page,
   * notifications) keep a slow poll. Defaults to no idle polling.
   */
  idlePollMs?: number;
  /** Set false to skip the query (e.g. outside local mode). */
  enabled?: boolean;
}

/** Jobs on this machine. Polls every 2s while any listed job is active. */
export const useJobs = (
  filter: JobListFilter = {},
  { idlePollMs, enabled = true }: UseJobsOptions = {},
) =>
  useQuery({
    queryKey: queryKeys.jobList(filter),
    enabled,
    queryFn: () => jobsApi.list(filter),
    placeholderData: keepPreviousData,
    refetchInterval: (query) =>
      query.state.data?.some(isActiveJob) ? 2000 : (idlePollMs ?? false),
  });

/** One job with its stored log. Polls every 2s while it is active. */
export const useJob = (id: string) =>
  useQuery({
    queryKey: queryKeys.job(id),
    queryFn: () => jobsApi.get(id),
    enabled: !!id,
    retry: (count, err) => httpStatus(err) !== 404 && count < 1,
    refetchInterval: (query) =>
      query.state.data && isActiveJob(query.state.data) ? 2000 : false,
  });

/** "Install version cancelled" (the toast after Cancel job). */
const cancelledToast = (job: JobSummary) =>
  toast.add({
    title: `${job.type_label} cancelled`,
    description: job.project_name
      ? `Nothing changed in ${job.project_name}.`
      : undefined,
  });

/**
 * Cancel job. Shows the prototype toasts: "<type> cancelled" / "Nothing
 * changed in <project>." on success, an error toast otherwise. A running
 * job usually reports "cancelled" at once; if it is still stopping, the
 * Jobs queries pick up the final state.
 */
export const useCancelJob = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => jobsApi.cancel(id),
    onSuccess: (job) => {
      if (job.status === 'cancelled') {
        cancelledToast(job);
      } else {
        toast.add({
          title: `Cancelling ${job.type_label.toLowerCase()}`,
          description: 'The job stops in a moment.',
          type: 'info',
        });
      }
    },
    onError: (err) => {
      toast.add({
        type: 'error',
        title: 'Couldn’t cancel the job',
        description:
          clientApiError(err)?.error ?? 'Refresh the Jobs page and try again.',
      });
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: queryKeys.jobs });
      qc.invalidateQueries({ queryKey: queryKeys.projects });
    },
  });
};

/** Current time, ticking every second (live durations, "Just now"). */
export function useNow(intervalMs = 1000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), intervalMs);
    return () => window.clearInterval(id);
  }, [intervalMs]);
  return now;
}

// ------------------------------------------------------------ log stream

export interface JobLogStream {
  /** Log received from the stream (the whole log so far), or null before the first chunk. */
  logs: string | null;
  /** True while the stream is open. */
  streaming: boolean;
}

/**
 * Follows a job's log over SSE (GET /client/jobs/:id/logs/stream) while
 * the job is active. The first chunk is the log so far; later chunks are
 * appended exactly as written. When the stream ends the job query is
 * refetched for the final status. If the stream fails the caller still has
 * the polled stored log (useJob), so this never surfaces an error.
 */
export function useJobLogStream(jobId: string, active: boolean): JobLogStream {
  const [logs, setLogs] = useState<string | null>(null);
  const [streaming, setStreaming] = useState(false);
  const qc = useQueryClient();

  useEffect(() => {
    if (!active || !jobId) {
      setStreaming(false);
      return;
    }
    const abort = new AbortController();
    setLogs(null);
    setStreaming(true);
    let text = '';

    const run = async () => {
      const headers: Record<string, string> = { Accept: 'text/event-stream' };
      try {
        const token = localStorage.getItem('auth_token');
        if (token) headers.Authorization = `Bearer ${token}`;
      } catch {
        // Storage unavailable; the local API does not need a token.
      }
      const response = await fetch(jobsApi.logStreamUrl(jobId), {
        headers,
        signal: abort.signal,
      });
      if (!response.ok || !response.body) {
        throw new Error(`log stream: HTTP ${response.status}`);
      }
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      let event = '';
      let data: string[] = [];
      // Returns true when the stream is over.
      const dispatch = () => {
        const payload = data.join('\n');
        const name = event;
        event = '';
        data = [];
        if (name === 'done' || name === 'error') return true;
        text += payload;
        setLogs(text);
        return false;
      };
      while (true) {
        const { value, done } = await reader.read();
        if (done) return;
        buffer += decoder.decode(value, { stream: true });
        let nl = buffer.indexOf('\n');
        while (nl !== -1) {
          let line = buffer.slice(0, nl);
          buffer = buffer.slice(nl + 1);
          if (line.endsWith('\r')) line = line.slice(0, -1);
          if (line === '') {
            if ((event || data.length) && dispatch()) {
              // Do not wait: cancelling can stall until the server closes.
              reader.cancel().catch(() => {});
              return;
            }
          } else if (line.startsWith('event:')) {
            event = line.slice(6).trim();
          } else if (line.startsWith('data:')) {
            const d = line.slice(5);
            data.push(d.startsWith(' ') ? d.slice(1) : d);
          }
          nl = buffer.indexOf('\n');
        }
      }
    };

    run()
      .catch(() => {
        // Fall back to the polled stored log.
      })
      .finally(() => {
        if (abort.signal.aborted) return;
        setStreaming(false);
        qc.invalidateQueries({ queryKey: queryKeys.job(jobId) });
        qc.invalidateQueries({ queryKey: queryKeys.jobs });
      });
    return () => abort.abort();
  }, [jobId, active, qc]);

  return { logs, streaming };
}

// ----------------------------------------------------- job progress toast

export interface JobToastOptions {
  /** Title while running, e.g. "Installing version 5". */
  title?: string;
  /** Description while running, e.g. "Pull and install job started". */
  description?: string;
  /** Title when the job succeeds, e.g. "Version 8 created". */
  successTitle?: string | ((job: JobSummary) => string);
  /** Description when the job succeeds, e.g. "Ready to use on this machine." */
  successDescription?: string | ((job: JobSummary) => string);
  /** Title when the job fails. */
  errorTitle?: string;
  /** Description when the job fails (default: the job's error). */
  errorDescription?: string | ((job: JobSummary) => string);
  /** Title when the job is cancelled (default "<title> cancelled"). */
  cancelledTitle?: string;
  /** Description when cancelled (default "Nothing changed in <project>."). */
  cancelledDescription?: string;
  /** Called once the job reaches a final state. */
  onSettled?: (job: JobSummary) => void;
  /**
   * Called when the user picks "View job" (navigate to /jobs/:id). Offered
   * while the job runs (unless it is cancellable) and when it fails.
   */
  onViewJob?: (job: JobSummary) => void;
  /** Show a "Cancel" action while the job runs (cancellable toast). */
  cancellable?: boolean;
  /** Poll interval in ms (default 1000). */
  intervalMs?: number;
}

const resolve = (
  v: string | ((job: JobSummary) => string) | undefined,
  job: JobSummary,
) => (typeof v === 'function' ? v(job) : v);

const tracked = new Map<string, () => void>();

/**
 * Shows a progress toast for a submitted job and follows it to the end:
 * a loading toast (with Cancel or View job), then success, cancelled or
 * failed (with View job). Usable outside React. Invalidates the projects
 * and jobs queries when the job settles. Tracking the same job twice
 * keeps the first toast. Returns a function that stops following the job.
 */
export function trackJobToast(job: JobSummary, opts: JobToastOptions = {}) {
  const existing = tracked.get(job.id);
  if (existing) return existing;

  const title = opts.title ?? job.type_label;
  const viewJob = (current: JobSummary) =>
    opts.onViewJob
      ? { children: 'View job', onClick: () => opts.onViewJob?.(current) }
      : undefined;
  const runningAction = (current: JobSummary) =>
    opts.cancellable
      ? {
          children: 'Cancel',
          onClick: () => {
            toast.update(id, {
              description: 'Cancelling…',
              actionProps: undefined,
            });
            jobsApi.cancel(current.id).catch((err) => {
              toast.update(id, {
                description:
                  clientApiError(err)?.error ?? 'Couldn’t cancel the job.',
                actionProps: viewJob(current),
              });
            });
          },
        }
      : viewJob(current);

  const id = toast.add({
    type: 'loading',
    title,
    description: opts.description,
    timeout: 0,
    actionProps: isActiveJob(job) ? runningAction(job) : undefined,
  });
  let stopped = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const stop = () => {
    stopped = true;
    if (timer) clearTimeout(timer);
    tracked.delete(job.id);
  };

  const settle = (final: JobSummary) => {
    stop();
    queryClient.invalidateQueries({ queryKey: queryKeys.projects });
    queryClient.invalidateQueries({ queryKey: queryKeys.jobs });
    if (final.status === 'completed') {
      toast.update(id, {
        type: 'success',
        title: resolve(opts.successTitle, final) ?? `${title} finished`,
        description: resolve(opts.successDescription, final),
        timeout: 5000,
        actionProps: undefined,
      });
    } else if (final.status === 'cancelled') {
      toast.update(id, {
        type: 'info',
        title: opts.cancelledTitle ?? `${title} cancelled`,
        description:
          opts.cancelledDescription ??
          (final.project_name
            ? `Nothing changed in ${final.project_name}.`
            : undefined),
        timeout: 5000,
        actionProps: undefined,
      });
    } else {
      const action = viewJob(final);
      toast.update(id, {
        type: 'error',
        title: opts.errorTitle ?? `${title} failed`,
        description: resolve(opts.errorDescription, final) ?? final.error,
        timeout: action ? 0 : 8000,
        actionProps: action,
      });
    }
    opts.onSettled?.(final);
  };

  if (!isActiveJob(job)) {
    settle(job);
    return () => {};
  }
  tracked.set(job.id, stop);
  const poll = async () => {
    if (stopped) return;
    try {
      const current = await jobsApi.get(job.id);
      if (stopped) return;
      if (!isActiveJob(current)) {
        settle(current);
        return;
      }
    } catch (err) {
      if (httpStatus(err) === 404) {
        // The job row is gone (e.g. its project was deleted).
        stop();
        toast.close(id);
        return;
      }
      // Keep polling; the local API is loopback and recovers quickly.
    }
    timer = setTimeout(poll, opts.intervalMs ?? 1000);
  };
  timer = setTimeout(poll, opts.intervalMs ?? 1000);
  return stop;
}

/** React wrapper around trackJobToast. */
export const useJobToast = () =>
  useCallback(
    (job: JobSummary, opts?: JobToastOptions) => trackJobToast(job, opts),
    [],
  );
