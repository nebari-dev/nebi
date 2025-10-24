import { useQuery } from '@tanstack/react-query';
import { jobsApi } from '@/api/jobs';

export const useJobs = () => {
  return useQuery({
    queryKey: ['jobs'],
    queryFn: jobsApi.list,
    refetchInterval: 2000, // Poll every 2 seconds for real-time updates
  });
};

export const useJob = (id: string) => {
  return useQuery({
    queryKey: ['jobs', id],
    queryFn: () => jobsApi.get(id),
    enabled: !!id,
    refetchInterval: 2000,
  });
};
