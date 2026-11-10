// Owner: U1 (projects).
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { projectsApi } from '@/api/client/projects';
import { queryKeys } from '@/lib/queryKeys';
import type {
  CreateProjectRequest,
  ManifestRenderRequest,
  PickFolderRequest,
} from '@/types/contracts';

/** Host capabilities (native folder picker in the desktop app). */
export const useCapabilities = () =>
  useQuery({
    queryKey: queryKeys.capabilities,
    queryFn: projectsApi.capabilities,
    staleTime: Number.POSITIVE_INFINITY,
  });

/** Local projects. Polls while any project has an active job. */
export const useProjects = () =>
  useQuery({
    queryKey: queryKeys.projects,
    queryFn: projectsApi.list,
    refetchInterval: (query) =>
      query.state.data?.some((p) => p.active_job_id) ? 2000 : false,
  });

export const useProject = (id: string) =>
  useQuery({
    queryKey: queryKeys.project(id),
    queryFn: () => projectsApi.get(id),
    enabled: !!id,
    refetchInterval: (query) =>
      query.state.data?.active_job_id ? 2000 : false,
  });

export const useCreateProject = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: CreateProjectRequest) => projectsApi.create(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.projects }),
  });
};

export const useDeleteProject = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => projectsApi.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.projects }),
  });
};

export const useCheckName = (name: string) =>
  useQuery({
    queryKey: queryKeys.nameCheck(name),
    queryFn: () => projectsApi.checkName(name),
    enabled: name.trim().length > 0,
  });

export const useParseManifest = () =>
  useMutation({
    mutationFn: (toml: string) => projectsApi.parseManifest(toml),
  });

export const useRenderManifest = () =>
  useMutation({
    mutationFn: (req: ManifestRenderRequest) => projectsApi.renderManifest(req),
  });

export const usePickFolder = () =>
  useMutation({
    mutationFn: (req: PickFolderRequest) => projectsApi.pickFolder(req),
  });
