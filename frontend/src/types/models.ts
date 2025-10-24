export interface User {
  id: string; // UUID
  username: string;
  email: string;
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
