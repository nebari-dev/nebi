export interface User {
  id: string; // UUID
  username: string;
  email: string;
  avatar_url?: string;
  is_admin?: boolean;
  created_at: string;
  updated_at: string;
}

export type ProjectStatus =
  | 'pending'
  | 'creating'
  | 'ready'
  | 'failed'
  | 'deleting';

// Derived environment install state; present only on local-mode servers.
export type InstallStatus =
  | 'not_installed'
  | 'installing'
  | 'installed'
  | 'uninstalling'
  | 'install_failed';

export interface Project {
  id: string; // UUID
  name: string;
  owner_id: string; // UUID
  owner?: User; // Optional owner details
  status: ProjectStatus;
  install_status?: InstallStatus;
  can_write?: boolean; // Effective write access, included in project details.
  created_at: string;
  updated_at: string;
  size_bytes?: number;
  size_formatted?: string;
  source?: 'local' | 'managed';
  path?: string;
  origin_name?: string;
  origin_tag?: string;
  origin_action?: string;
}

export interface CreateProjectRequest {
  name?: string;
  pixi_toml?: string;
  path?: string;
  source?: 'local' | 'managed';
}

export type JobType =
  | 'create'
  | 'delete'
  | 'install'
  | 'remove'
  | 'update'
  | 'rollback'
  | 'env_install'
  | 'env_uninstall';
export type JobStatus = 'pending' | 'running' | 'completed' | 'failed';
export type JsonValue =
  | string
  | number
  | boolean
  | null
  | JsonValue[]
  | { [key: string]: JsonValue };

export interface Job {
  id: string; // UUID
  project_id: string; // UUID
  type: JobType;
  status: JobStatus;
  logs: string;
  error?: string;
  metadata?: Record<string, JsonValue>;
  created_at: string;
  started_at?: string;
  completed_at?: string;
}

export interface Package {
  id: string; // UUID
  project_id: string; // UUID
  name: string;
  version?: string;
  installed_at: string;
}

export interface InstallPackagesRequest {
  packages: string[];
}

// GET /auth/config. 'oidc' means the SPA signs in directly against the
// identity provider (authorization code + PKCE) and sends the IdP access token
// as a bearer token; 'none' means requests need no credentials (local mode, or
// a team server running with auth disabled).
export interface OidcAuthConfig {
  type: 'oidc';
  issuer_url: string;
  client_id: string;
  scopes: string[];
}

export type AuthConfig = OidcAuthConfig | { type: 'none' };

export interface AuditLog {
  id: string;
  user_id: string;
  action: string;
  resource: string;
  resource_id?: string;
  details_json?: Record<string, JsonValue>;
  timestamp: string;
  user?: User;
}

export type Collaborator =
  | {
      kind: 'user';
      user_id: string;
      username: string;
      email: string;
      role: 'owner' | 'editor' | 'viewer';
      is_owner: boolean;
    }
  | GroupCollaborator;

export interface ShareProjectRequest {
  user_id: string;
  role: 'editor' | 'viewer';
}

export interface ProjectVersion {
  id: string; // UUID
  project_id: string; // UUID
  version_number: number;
  manifest_version?: string;
  lock_file_content?: string; // Not included in list view
  manifest_content?: string; // Not included in list view
  package_metadata?: string; // Not included in list view
  job_id?: string; // UUID
  created_by: string; // UUID
  description?: string;
  created_at: string;
}

export interface RollbackRequest {
  version_number: number;
}

export interface DashboardStats {
  total_disk_usage_bytes: number;
  total_disk_usage_formatted: string;
}

// OCI Registry types
export interface OCIRegistry {
  id: string; // UUID
  name: string;
  url: string;
  username: string;
  has_api_token: boolean;
  is_default: boolean;
  namespace: string;
  config_managed: boolean;
  restricted: boolean;
  created_at: string;
}

export interface CreateRegistryRequest {
  name: string;
  url: string;
  username?: string;
  password?: string;
  api_token?: string;
  is_default?: boolean;
  namespace?: string;
  restricted?: boolean;
}

export interface UpdateRegistryRequest {
  name?: string;
  url?: string;
  username?: string;
  password?: string;
  api_token?: string;
  is_default?: boolean;
  namespace?: string;
  restricted?: boolean;
}

// Project Tag types
export interface ProjectTag {
  tag: string;
  version_number: number;
  created_at: string;
  updated_at: string;
}

// Publication types
export interface Publication {
  id: string; // UUID
  registry_name: string;
  registry_url: string;
  registry_namespace: string;
  repository: string;
  tag: string;
  digest: string;
  is_public: boolean;
  published_by: string;
  published_at: string;
}

export interface PublishDefaults {
  registry_id: string;
  registry_name: string;
  namespace: string;
  repository: string;
  tag: string;
}

export interface PublishRequest {
  registry_id: string; // UUID
  repository: string;
  tag: string;
}

// Remote server types
export interface RemoteServer {
  id?: string;
  url: string;
  username: string;
  status: 'connected' | 'disconnected';
}

export interface ConnectServerRequest {
  url: string;
}

// An OAuth device authorization started by the local backend against the
// remote server's identity provider.
export interface DeviceAuthorization {
  user_code: string;
  verification_uri: string;
  verification_uri_complete?: string;
  expires_in: number;
  interval: number;
}

export interface RemoteConnected {
  status: 'connected';
  url: string;
  username: string;
}

// POST /remote/connect: a device authorization to approve, or an immediate
// connection when the remote server has auth disabled.
export type RemoteConnectStartResponse = DeviceAuthorization | RemoteConnected;

// POST /remote/connect/poll. Terminal failures (expired or denied: 400, no
// connection in progress: 409) are returned as 4xx errors instead.
export type RemoteConnectPollResponse =
  | { status: 'pending'; interval?: number }
  | RemoteConnected;

export const isRemoteConnected = (
  response: RemoteConnectStartResponse | RemoteConnectPollResponse,
): response is RemoteConnected =>
  'status' in response && response.status === 'connected';

export interface RemoteProject {
  id: string;
  name: string;
  status: string;
  size_bytes: number;
  owner?: {
    id: string;
    username: string;
    email: string;
  };
  created_at: string;
  updated_at: string;
}

export interface RemoteProjectVersion {
  id: string;
  project_id: string;
  version_number: number;
  manifest_version?: string;
  created_at: string;
  description?: string;
}

export interface RemoteProjectTag {
  tag: string;
  version_number: number;
  created_at: string;
  updated_at: string;
}

export interface CreateRemoteProjectRequest {
  name: string;
  pixi_toml?: string;
}

// Registry browse types
export interface RegistryRepository {
  name: string;
  is_public?: boolean;
}

export interface RegistryTag {
  name: string;
}

export interface ImportEnvironmentRequest {
  repository?: string;
  repository_path?: string;
  tag: string;
  name: string;
}

// Group types. Groups come from the identity provider's groups claim.
export interface Group {
  id: string;
  name: string;
  created_at: string;
  updated_at: string;
}

export interface GroupWithMemberCount extends Group {
  member_count: number;
}

export interface GroupMember {
  group_id: string;
  user_id: string;
  created_at: string;
  user?: User;
}

export interface GroupCollaborator {
  kind: 'group';
  group_id: string;
  name: string;
  role: 'editor' | 'viewer';
  is_owner: false;
}

export interface ShareProjectWithGroupRequest {
  group_id: string;
  role: 'editor' | 'viewer';
}
