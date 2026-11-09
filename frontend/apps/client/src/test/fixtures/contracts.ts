// Contract fixtures for the client API (issue #649). Frozen: shared by every
// unit's tests and MSW handlers. Override fields with spreads in tests.
import type {
  AccessList,
  Capabilities,
  JobDetail,
  JobSummary,
  ProjectListItem,
  RegistryItem,
  ServerConnection,
  ServerProjectItem,
  VersionDetail,
  VersionItem,
} from '@/types/contracts';

export const PROJECT_ID = '11111111-1111-4111-8111-111111111111';
export const JOB_ID = '22222222-2222-4222-8222-222222222222';
export const REGISTRY_ID = '33333333-3333-4333-8333-333333333333';
export const SERVER_PROJECT_ID = '44444444-4444-4444-8444-444444444444';

export const mockCapabilities: Capabilities = {
  native_folder_picker: false,
  platform: 'linux/amd64',
  default_projects_dir: '/home/user/.local/share/nebi/projects',
};

export const mockProjectItem: ProjectListItem = {
  id: PROJECT_ID,
  name: 'Project_1',
  path: '/home/user/Project_1',
  status: 'ready',
  install_status: 'installed',
  installed_version: 7,
  last_installed_version: 7,
  latest_version: 7,
  size_bytes: 1_200_000_000,
  size: '1.2 GB',
  remotes: [
    {
      kind: 'server',
      name: 'team-nebi',
      url: 'https://nebi.example.com',
      server_project_id: SERVER_PROJECT_ID,
    },
    { kind: 'registry', name: 'ghcr.io/example-org' },
  ],
  update_available: false,
  server_latest_version: 6,
  failed_install_job_id: null,
  active_job_id: null,
  created_at: '2026-06-01T00:00:00Z',
  updated_at: '2026-09-29T00:00:00Z',
};

export const mockVersionItem: VersionItem = {
  number: 7,
  description: 'Added scipy for the new solver',
  author: { name: 'Sam Garcia', username: 'sam' },
  created_at: '2026-09-29T00:00:00Z',
  tags: ['latest'],
  base_version: 6,
  published: [],
  sync: 'newest_here',
  server_only: false,
  installed: true,
  content_hash: 'sha-0123456789ab',
};

export const mockVersionDetail: VersionDetail = {
  ...mockVersionItem,
  spec: {
    channels: ['conda-forge', 'defaults'],
    platforms: ['linux-64', 'linux-aarch64'],
    requested: [
      { name: 'python', constraint: '3.12.*' },
      { name: 'numpy', constraint: '>=2.0' },
      { name: 'scipy', constraint: '>=1.11' },
    ],
  },
};

export const mockJobSummary: JobSummary = {
  id: JOB_ID,
  type: 'install_version',
  type_label: 'Install version',
  status: 'completed',
  project_id: PROJECT_ID,
  project_name: 'Project_1',
  version_number: 7,
  created_at: '2026-09-29T00:00:00Z',
  started_at: '2026-09-29T00:00:01Z',
  completed_at: '2026-09-29T00:00:31Z',
  duration_ms: 30_000,
};

export const mockJobDetail: JobDetail = {
  ...mockJobSummary,
  logs: 'Running: pixi install\n',
};

export const mockServerConnection: ServerConnection = {
  connected: true,
  url: 'https://nebi.example.com',
  name: 'nebi.example.com',
  username: 'sam',
  auth: 'token',
  last_synced_at: '2026-09-29T00:00:00Z',
};

export const mockDisconnected: ServerConnection = {
  connected: false,
  url: '',
  name: '',
  username: '',
  auth: 'none',
  last_synced_at: null,
};

export const mockServerProject: ServerProjectItem = {
  id: SERVER_PROJECT_ID,
  name: 'ml-baseline',
  owner: 'alex',
  namespace: 'team-nebi',
  latest_version: 3,
  updated_at: '2026-09-28T00:00:00Z',
  role: 'edit',
  local_project_id: null,
};

export const mockAccessList: AccessList = {
  entries: [
    {
      principal: {
        kind: 'user',
        id: 'u-sam',
        name: 'Sam Garcia',
        email: 'sam@example.com',
      },
      role: 'owner',
    },
  ],
  can_manage: true,
};

export const mockRegistryItem: RegistryItem = {
  id: REGISTRY_ID,
  name: 'GitHub Container Registry',
  url: 'ghcr.io',
  namespace: 'example-org',
  username: 'sam',
  has_credentials: true,
  is_default: true,
  can_unpublish: false,
  unpublish_reason: 'GitHub Container Registry does not support deleting tags',
};
