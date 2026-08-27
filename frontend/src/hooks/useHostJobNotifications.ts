import { useQuery } from '@tanstack/react-query';
import { useEffect, useRef } from 'react';
import { jobsApi } from '@/api/jobs';
import {
  isEmbedded,
  NEBI_JOB_COMPLETED_MESSAGE,
  postToHost,
} from '@/lib/hostBridge';
import { useModeStore } from '@/store/modeStore';
import type { JobStatus } from '@/types';

export function useHostJobNotifications(): void {
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const enabled = isEmbedded() && isLocalMode;
  const previousStatuses = useRef<Record<string, JobStatus>>({});
  const initialized = useRef(false);

  const { data: jobs } = useQuery({
    queryKey: ['jobs'],
    queryFn: jobsApi.list,
    refetchInterval: 2000,
    enabled,
  });

  useEffect(() => {
    if (!enabled) {
      previousStatuses.current = {};
      initialized.current = false;
      return;
    }
    if (!jobs) {
      return;
    }

    const nextStatuses: Record<string, JobStatus> = {};

    for (const job of jobs) {
      const previousStatus = previousStatuses.current[job.id];
      nextStatuses[job.id] = job.status;

      if (
        initialized.current &&
        previousStatus !== 'completed' &&
        job.status === 'completed'
      ) {
        postToHost({
          type: NEBI_JOB_COMPLETED_MESSAGE,
          jobType: job.type,
          status: job.status,
          workspaceId: job.workspace_id,
        });
      }
    }

    previousStatuses.current = nextStatuses;
    initialized.current = true;
  }, [enabled, jobs]);
}
