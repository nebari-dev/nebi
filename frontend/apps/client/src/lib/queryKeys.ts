import type { JobListFilter } from '@/types/contracts';

// TanStack Query keys for the client API (issue #649). Frozen: every unit
// uses these so cross-unit invalidation works (e.g. an install job settling
// invalidates projects and versions). Prefix-invalidate with the shorter
// keys: invalidateQueries({ queryKey: queryKeys.projects }) also refreshes
// every project detail and version query below it.
export const queryKeys = {
  capabilities: ['client', 'capabilities'] as const,
  projects: ['client', 'projects'] as const,
  project: (id: string) => ['client', 'projects', id] as const,
  versions: (id: string) => ['client', 'projects', id, 'versions'] as const,
  version: (id: string, n: number) =>
    ['client', 'projects', id, 'versions', n] as const,
  versionFile: (id: string, n: number, name: string) =>
    ['client', 'projects', id, 'versions', n, 'files', name] as const,
  resolvedPackages: (id: string, n: number) =>
    ['client', 'projects', id, 'versions', n, 'packages'] as const,
  publishDefaults: (id: string, n: number) =>
    ['client', 'projects', id, 'versions', n, 'publish-defaults'] as const,
  compare: (id: string, base: number, target: number) =>
    ['client', 'projects', id, 'compare', base, target] as const,
  nameCheck: (name: string) => ['client', 'name-check', name] as const,
  serverConnection: ['client', 'remote', 'server'] as const,
  serverProjects: (search: string) =>
    ['client', 'remote', 'projects', search] as const,
  remoteDetails: (serverProjectId: string) =>
    ['client', 'remote', 'details', serverProjectId] as const,
  access: (serverProjectId: string) =>
    ['client', 'remote', 'access', serverProjectId] as const,
  principals: (q: string) => ['client', 'remote', 'principals', q] as const,
  registries: ['client', 'registries'] as const,
  registryRepositories: (id: string, search: string) =>
    ['client', 'registries', id, 'repositories', search] as const,
  registryTags: (id: string, repository: string) =>
    ['client', 'registries', id, 'tags', repository] as const,
  jobs: ['client', 'jobs'] as const,
  jobList: (filter: JobListFilter) =>
    ['client', 'jobs', 'list', filter] as const,
  job: (id: string) => ['client', 'jobs', 'detail', id] as const,
};
