import { useQuery } from '@tanstack/react-query';
import { useEffect, useRef } from 'react';
import { jobsApi } from '@/api/jobs';
import type { Job, JobStatus } from '@/types';

const parentRefreshJobTypes = new Set<Job['type']>(['create', 'env_install']);

function useParentJobCompletionMessage(jobs: Job | Job[] | undefined): void {
  const previousStatuses = useRef<Record<string, JobStatus>>({});

  useEffect(() => {
    const nextStatuses: Record<string, JobStatus> = {};
    const jobList = Array.isArray(jobs) ? jobs : jobs ? [jobs] : [];

    for (const job of jobList) {
      const previousStatus = previousStatuses.current[job.id];
      nextStatuses[job.id] = job.status;

      if (
        previousStatus !== undefined &&
        previousStatus !== 'completed' &&
        job.status === 'completed' &&
        parentRefreshJobTypes.has(job.type)
      ) {
        const jobType = job.type;
        const workspace = job.workspace_id;

        if (window.parent !== window) {
          window.parent.postMessage(
            { type: 'nebi:job-completed', jobType, workspace },
            window.location.origin,
          );
        }
      }
    }

    previousStatuses.current = nextStatuses;
  }, [jobs]);
}

export const useJobs = () => {
  const query = useQuery({
    queryKey: ['jobs'],
    queryFn: jobsApi.list,
    refetchInterval: 2000, // Poll every 2 seconds for real-time updates
  });

  useParentJobCompletionMessage(query.data);

  return query;
};

export const useJob = (id: string) => {
  const query = useQuery({
    queryKey: ['jobs', id],
    queryFn: () => jobsApi.get(id),
    enabled: !!id,
    refetchInterval: 2000,
  });

  useParentJobCompletionMessage(query.data);

  return query;
};
