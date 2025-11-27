export interface User {
  id: string; // UUID
  username: string;
  email: string;
  is_admin?: boolean;
  created_at: string;
  updated_at: string;
}

export type EnvironmentStatus = 'pending' | 'creating' | 'ready' | 'failed' | 'deleting';

export interface Environment {
  id: string; // UUID
  name: string;
  owner_id: string; // UUID
  status: EnvironmentStatus;
  package_manager: string;
  created_at: string;
  updated_at: string;
  size_bytes: number;
  size_formatted: string;
}

export interface CreateEnvironmentRequest {
  name: string;
  package_manager?: string;
  pixi_toml?: string;
}

export type JobType = 'create' | 'delete' | 'install' | 'remove' | 'update';
export type JobStatus = 'pending' | 'running' | 'completed' | 'failed';

export interface Job {
  id: string; // UUID
  environment_id: string; // UUID
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
  environment_id: string; // UUID
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

export interface ShareEnvironmentRequest {
  user_id: string;
  role: 'editor' | 'viewer';
}

export interface EnvironmentVersion {
  id: string; // UUID
  environment_id: string; // UUID
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
