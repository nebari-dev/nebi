export interface User {
  id: string; // UUID
  username: string;
  email: string;
  avatar_url?: string;
  is_admin?: boolean;
  created_at: string;
  updated_at: string;
}

export type WorkspaceStatus = 'pending' | 'creating' | 'ready' | 'failed' | 'deleting';

export interface Workspace {
  id: string; // UUID
  name: string;
  owner_id: string; // UUID
  owner?: User; // Optional owner details
  status: WorkspaceStatus;
  package_manager: string;
  created_at: string;
  updated_at: string;
  size_bytes: number;
  size_formatted: string;
  source?: 'local' | 'managed';
  path?: string;
  origin_name?: string;
  origin_tag?: string;
  origin_action?: string;
}

export interface CreateWorkspaceRequest {
  name: string;
  package_manager?: string;
  pixi_toml?: string;
  path?: string;
  source?: 'local' | 'managed';
}

export type JobType = 'create' | 'delete' | 'install' | 'remove' | 'update';
export type JobStatus = 'pending' | 'running' | 'completed' | 'failed';

export interface Job {
  id: string; // UUID
  workspace_id: string; // UUID
  type: JobType;
  status: JobStatus;
  logs: string;
  error?: string;
  metadata?: Record<string, any>;
  created_at: string;
  started_at?: string;
  completed_at?: string;
}

export interface Package {
  id: string; // UUID
  workspace_id: string; // UUID
  name: string;
  version?: string;
  installed_at: string;
}

export interface InstallPackagesRequest {
  packages: string[];
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  token: string;
  user: User;
}

export interface CreateUserRequest {
  username: string;
  email: string;
  password: string;
  is_admin?: boolean;
}

export interface AuditLog {
  id: string;
  user_id: string;
  action: string;
  resource: string;
  resource_id?: string;
  details_json?: Record<string, any>;
  timestamp: string;
  user?: User;
}

export interface Collaborator {
  user_id: string;
  username: string;
  email: string;
  role: 'owner' | 'editor' | 'viewer';
  is_owner: boolean;
}

export interface ShareWorkspaceRequest {
  user_id: string;
  role: 'editor' | 'viewer';
}

export interface WorkspaceVersion {
  id: string; // UUID
  workspace_id: string; // UUID
  version_number: number;
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
  default_repository: string;
  created_at: string;
}

export interface CreateRegistryRequest {
  name: string;
  url: string;
  username?: string;
  password?: string;
  api_token?: string;
  is_default?: boolean;
  default_repository?: string;
}

export interface UpdateRegistryRequest {
  name?: string;
  url?: string;
  username?: string;
  password?: string;
  api_token?: string;
  is_default?: boolean;
  default_repository?: string;
}

// Workspace Tag types
export interface WorkspaceTag {
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
  repository: string;
  tag: string;
  digest: string;
  published_by: string;
  published_at: string;
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
  username: string;
  password: string;
}

export interface RemoteWorkspace {
  id: string;
  name: string;
  status: string;
  package_manager: string;
  size_bytes: number;
  owner?: {
    id: string;
    username: string;
    email: string;
  };
  created_at: string;
  updated_at: string;
}

export interface RemoteWorkspaceVersion {
  id: string;
  workspace_id: string;
  version_number: number;
  created_at: string;
}

export interface RemoteWorkspaceTag {
  tag: string;
  version_number: number;
  created_at: string;
  updated_at: string;
}

export interface CreateRemoteWorkspaceRequest {
  name: string;
  package_manager?: string;
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
  repository: string;
  tag: string;
  name: string;
}
