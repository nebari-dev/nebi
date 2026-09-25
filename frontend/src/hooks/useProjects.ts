import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { projectsApi } from '@/api/projects';
import type { CreateProjectRequest } from '@/types';

export const useProjects = () => {
  return useQuery({
    queryKey: ['projects'],
    queryFn: projectsApi.list,
    refetchInterval: 2000, // Poll every 2 seconds for status updates
  });
};

export const useProject = (id: string) => {
  return useQuery({
    queryKey: ['projects', id],
    queryFn: () => projectsApi.get(id),
    enabled: !!id,
    refetchInterval: 2000,
  });
};

export const useCreateProject = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateProjectRequest) => projectsApi.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
    },
  });
};

export const useDeleteProject = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => projectsApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
    },
  });
};

export const useInstallProject = (id: string) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => projectsApi.install(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};

export const useUninstallProject = (id: string) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => projectsApi.uninstall(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
};

export const useProjectTags = (id: string) => {
  return useQuery({
    queryKey: ['projects', id, 'tags'],
    queryFn: () => projectsApi.listTags(id),
    enabled: !!id,
  });
};
