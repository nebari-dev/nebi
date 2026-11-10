// Owner: U2 (detail).
import { toast } from '@nebi/ui/components/ui/toast';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useRef } from 'react';
import { create } from 'zustand';
import { clientApiError } from '@/api/client/base';
import { detailApi } from '@/api/client/detail';
import { installErrorText } from '@/components/detail/format';
import { useJob } from '@/hooks/client/jobs';
import { queryKeys } from '@/lib/queryKeys';
import {
  type InstallRequest,
  isActiveJob,
  type JobSummary,
  type ProjectListItem,
} from '@/types/contracts';

export const useVersions = (projectId: string) =>
  useQuery({
    queryKey: queryKeys.versions(projectId),
    queryFn: () => detailApi.versions(projectId),
    enabled: !!projectId,
  });

export const useVersion = (projectId: string, n: number | undefined) =>
  useQuery({
    queryKey: queryKeys.version(projectId, n ?? 0),
    queryFn: () => detailApi.version(projectId, n as number),
    enabled: !!projectId && !!n,
  });

export const useVersionFile = (
  projectId: string,
  n: number | undefined,
  name: 'pixi.toml' | 'pixi.lock',
  enabled = true,
) =>
  useQuery({
    queryKey: queryKeys.versionFile(projectId, n ?? 0, name),
    queryFn: () => detailApi.file(projectId, n as number, name),
    enabled: enabled && !!projectId && !!n,
  });

export const useResolvedPackages = (projectId: string, n: number | undefined) =>
  useQuery({
    queryKey: queryKeys.resolvedPackages(projectId, n ?? 0),
    queryFn: () => detailApi.resolvedPackages(projectId, n as number),
    enabled: !!projectId && !!n,
  });

/** Install a version (no version = last installed, else latest). */
export const useInstall = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      projectId,
      ...req
    }: InstallRequest & { projectId: string }) =>
      detailApi.install(projectId, req),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.projects }),
  });
};

export const useUninstall = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (projectId: string) => detailApi.uninstall(projectId),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.projects }),
  });
};

/** Downloads a version's .tar.gz through the browser. */
export const useDownloadArchive = () =>
  useMutation({
    mutationFn: async ({ projectId, n }: { projectId: string; n: number }) => {
      const { blob, fileName } = await detailApi.downloadArchive(projectId, n);
      const url = URL.createObjectURL(blob);
      try {
        const a = document.createElement('a');
        a.href = url;
        a.download = fileName;
        document.body.appendChild(a);
        a.click();
        a.remove();
      } finally {
        setTimeout(() => URL.revokeObjectURL(url), 1000);
      }
      return fileName;
    },
  });

// ------------------------------------------------- install progress state

/** One install started from the detail page. */
export interface InstallRun {
  version: number;
  jobId: string;
}

interface InstallProgressState {
  /** Running installs submitted from this window, per project. */
  running: Record<string, InstallRun>;
  /** Finished installs whose success banner is showing, per project. */
  done: Record<string, InstallRun>;
  /** Failed installs (this window), per project. */
  failed: Record<string, InstallRun & { error?: string }>;
  startRun: (projectId: string, run: InstallRun) => void;
  finishRun: (projectId: string, job: JobSummary) => void;
  dismissDone: (projectId: string) => void;
}

const without = <T>(rec: Record<string, T>, key: string) => {
  const { [key]: _drop, ...rest } = rec;
  return rest;
};

export const useInstallProgressStore = create<InstallProgressState>((set) => ({
  running: {},
  done: {},
  failed: {},
  startRun: (projectId, run) =>
    set((s) => ({
      running: { ...s.running, [projectId]: run },
      done: without(s.done, projectId),
      failed: without(s.failed, projectId),
    })),
  finishRun: (projectId, job) =>
    set((s) => {
      const run = s.running[projectId];
      const version = job.version_number ?? run?.version;
      const next = { running: without(s.running, projectId) };
      if (version == null) return next;
      if (job.status === 'completed') {
        return {
          ...next,
          done: { ...s.done, [projectId]: { version, jobId: job.id } },
        };
      }
      if (job.status === 'failed') {
        return {
          ...next,
          failed: {
            ...s.failed,
            [projectId]: { version, jobId: job.id, error: job.error },
          },
        };
      }
      return next;
    }),
  dismissDone: (projectId) =>
    set((s) => ({ done: without(s.done, projectId) })),
}));

/** How long the "installed" banner stays before it hides itself. */
export const INSTALL_DONE_BANNER_MS = 8000;

export interface ProjectInstallState {
  /** Version being installed (and its job), from this window or another. */
  installing: InstallRun | null;
  /** An uninstall job is running. */
  uninstalling: boolean;
  /** Just finished installing (success banner). */
  done: InstallRun | null;
  /** The last install failed on this version (failure alert). */
  failed: (InstallRun & { error?: string }) | null;
  /** Any job is running on the project. */
  busy: boolean;
}

/**
 * Follows the project's install/uninstall job: an install submitted from
 * this window (install progress store) or the project's active job (e.g.
 * started from the projects list or the CLI). Failed installs come from
 * this window or from the project's failed_install_job_id.
 */
export function useProjectInstallState(
  project: ProjectListItem | undefined,
  onViewJob?: (jobId: string) => void,
): ProjectInstallState {
  const projectId = project?.id ?? '';
  const qc = useQueryClient();
  const onViewJobRef = useRef(onViewJob);
  onViewJobRef.current = onViewJob;
  const run = useInstallProgressStore((s) => s.running[projectId]);
  const done = useInstallProgressStore((s) => s.done[projectId]);
  const localFailed = useInstallProgressStore((s) => s.failed[projectId]);
  const finishRun = useInstallProgressStore((s) => s.finishRun);
  const dismissDone = useInstallProgressStore((s) => s.dismissDone);

  const followedId = run?.jobId ?? project?.active_job_id ?? '';
  const { data: job } = useJob(followedId);
  const followed = job && job.id === followedId ? job : undefined;

  // A finished install from this window: record it and refresh.
  useEffect(() => {
    if (!run || !followed || followed.id !== run.jobId) return;
    if (isActiveJob(followed)) return;
    finishRun(projectId, followed);
    qc.invalidateQueries({ queryKey: queryKeys.projects });
    qc.invalidateQueries({ queryKey: queryKeys.jobs });
    if (followed.status === 'failed') {
      const version = followed.version_number ?? run.version;
      toast.add({
        type: 'error',
        title: `Version ${version} couldn’t install`,
        description: installErrorText(followed.error, version),
        actionProps: {
          children: 'View job',
          onClick: () => onViewJobRef.current?.(followed.id),
        },
      });
    }
  }, [run, followed, finishRun, projectId, qc]);

  // The success banner hides itself after a while.
  useEffect(() => {
    if (!done) return;
    const t = window.setTimeout(
      () => dismissDone(projectId),
      INSTALL_DONE_BANNER_MS,
    );
    return () => window.clearTimeout(t);
  }, [done, dismissDone, projectId]);

  const failedJobId = project?.failed_install_job_id ?? '';
  const { data: failedJob } = useJob(localFailed ? '' : failedJobId);

  return useMemo(() => {
    const activeJob = followed && isActiveJob(followed) ? followed : undefined;
    let installing: InstallRun | null = null;
    if (run && (!followed || activeJob)) {
      installing = run;
    } else if (activeJob && activeJob.type === 'install_version') {
      installing = {
        version: activeJob.version_number ?? 0,
        jobId: activeJob.id,
      };
    }
    let failed: ProjectInstallState['failed'] = localFailed ?? null;
    if (
      !failed &&
      failedJob &&
      failedJob.id === failedJobId &&
      failedJob.version_number != null
    ) {
      failed = {
        version: failedJob.version_number,
        jobId: failedJob.id,
        error: failedJob.error,
      };
    }
    return {
      installing,
      uninstalling: activeJob?.type === 'uninstall',
      done: done ?? null,
      failed: installing ? null : failed,
      busy: !!installing || !!activeJob || !!project?.active_job_id,
    };
  }, [run, followed, localFailed, failedJob, failedJobId, done, project]);
}

/** Starts installing a version and follows it on the page. */
export function useInstallVersion(project: ProjectListItem | undefined) {
  const install = useInstall();
  const startRun = useInstallProgressStore((s) => s.startRun);
  return {
    isPending: install.isPending,
    install: (version: number) => {
      if (!project) return;
      install.mutate(
        { projectId: project.id, version },
        {
          onSuccess: (job) => {
            startRun(project.id, {
              version: job.version_number ?? version,
              jobId: job.id,
            });
          },
          onError: (err) => {
            toast.add({
              type: 'error',
              title: `Version ${version} couldn’t install`,
              description:
                clientApiError(err)?.error ?? 'The install could not start.',
            });
          },
        },
      );
    },
  };
}
