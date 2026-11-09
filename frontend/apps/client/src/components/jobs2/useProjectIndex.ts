// Owner: U8 (jobs). Which projects still exist on this machine, for the
// job -> project links ("View project" is disabled for deleted projects).
import { useMemo } from 'react';
import { useProjects } from '@/hooks/client/projects';
import type { ProjectListItem } from '@/types/contracts';

export interface ProjectIndex {
  projects: ProjectListItem[];
  /**
   * False only when the project list loaded and the project is not in it.
   * While the list is loading or unavailable, links stay enabled and the
   * project page handles a missing project.
   */
  exists: (projectId: string) => boolean;
}

export function useProjectIndex(): ProjectIndex {
  const { data } = useProjects();
  return useMemo(() => {
    const projects = data ?? [];
    const ids = new Set(projects.map((p) => p.id));
    return {
      projects,
      exists: (id: string) => (data ? ids.has(id) : true),
    };
  }, [data]);
}
