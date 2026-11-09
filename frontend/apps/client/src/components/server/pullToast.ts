// Owner: U5 (remote). The pull / pull-and-install job toast (prototype
// PullToast): "<project>" + "Pull and install job started", then
// "Pulled. Installing version N…" once the versions are on this machine,
// then "<project> installed" / "<project> pulled", or the cancelled copy.
import { toast } from '@nebi/ui/components/ui/toast';
import { jobsApi } from '@/api/client/jobs';
import { queryClient } from '@/lib/queryClient';
import { queryKeys } from '@/lib/queryKeys';
import { isActiveJob, type JobSummary } from '@/types/contracts';

export interface PullToastOptions {
  /** Server project name, the toast title ("ml-baseline"). */
  projectName: string;
  /** True for pull_install. */
  install: boolean;
  /** "Open project" action on the final toast. */
  onOpenProject?: () => void;
  /** "View job" action (failures, and while installing). */
  onViewJob?: (job: JobSummary) => void;
  /** The versions are on this machine and the install started. */
  onInstalling?: (job: JobSummary) => void;
  /** The job reached a final state. */
  onSettled?: (job: JobSummary) => void;
  /** Poll interval in ms (default 1000). */
  intervalMs?: number;
}

/**
 * Follows a pull / pull_install job with one toast. Returns a function that
 * stops following it.
 */
export function trackPullToast(job: JobSummary, opts: PullToastOptions) {
  const title = opts.projectName;
  const cancel = {
    children: 'Cancel',
    onClick: () => {
      void jobsApi.cancel(job.id).catch(() => {});
    },
  };
  const id = toast.add({
    type: 'loading',
    title,
    description: opts.install
      ? 'Pull and install job started'
      : 'Pull job started',
    timeout: 0,
    actionProps: cancel,
  });
  const openProject = opts.onOpenProject
    ? { children: 'Open project', onClick: () => opts.onOpenProject?.() }
    : undefined;
  let installing = false;
  let stopped = false;

  const settle = (final: JobSummary) => {
    stopped = true;
    queryClient.invalidateQueries({ queryKey: queryKeys.projects });
    queryClient.invalidateQueries({ queryKey: queryKeys.jobs });
    queryClient.invalidateQueries({
      queryKey: ['client', 'remote', 'projects'],
    });
    const n = final.version_number;
    const pulled = n != null;
    const viewJob = opts.onViewJob
      ? { children: 'View job', onClick: () => opts.onViewJob?.(final) }
      : undefined;
    if (final.status === 'completed') {
      toast.update(id, {
        type: 'success',
        title: opts.install ? `${title} installed` : `${title} pulled`,
        description: opts.install
          ? `Version ${n} is ready to use on this machine.`
          : `Version ${n} is on this machine. Install it when you need it.`,
        actionProps: openProject,
        timeout: openProject ? 0 : 5000,
      });
    } else if (final.status === 'cancelled') {
      toast.update(id, {
        type: 'info',
        title: pulled ? `${title} pulled` : `Pull of ${title} cancelled`,
        description: pulled
          ? `The install was cancelled. Version ${n} is on this machine; install it when you need it.`
          : 'Nothing was added to this machine.',
        actionProps: pulled ? openProject : undefined,
        timeout: pulled && openProject ? 0 : 5000,
      });
    } else {
      toast.update(id, {
        type: 'error',
        title: pulled
          ? `Version ${n} couldn’t install`
          : `${title} couldn’t be pulled`,
        description: final.error,
        actionProps: viewJob,
        timeout: viewJob ? 0 : 8000,
      });
    }
    opts.onSettled?.(final);
  };

  const step = (current: JobSummary) => {
    if (!isActiveJob(current)) {
      settle(current);
      return true;
    }
    if (
      opts.install &&
      !installing &&
      current.status === 'running' &&
      current.version_number != null
    ) {
      installing = true;
      toast.update(id, {
        type: 'loading',
        title,
        description: `Pulled. Installing version ${current.version_number}…`,
        actionProps: cancel,
        timeout: 0,
      });
      opts.onInstalling?.(current);
    }
    return false;
  };

  if (step(job)) return () => {};
  const timer = setInterval(async () => {
    if (stopped) return;
    try {
      const current = await jobsApi.get(job.id);
      if (step(current)) clearInterval(timer);
    } catch {
      // Keep polling; the local API is loopback and recovers quickly.
    }
  }, opts.intervalMs ?? 1000);
  return () => {
    stopped = true;
    clearInterval(timer);
  };
}
