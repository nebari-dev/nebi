// Team-server sharing and catalog types (issue #649), matching
// internal/server/core/types.go. The client app mirrors the same shapes in
// apps/client/src/types/contracts.ts.

export type ShareRole = 'owner' | 'edit' | 'view';

export interface Principal {
  kind: 'user' | 'group';
  id: string;
  name: string;
  email?: string;
  members?: number;
}

export interface AccessEntry {
  principal: Principal;
  role: ShareRole;
}

export interface AccessList {
  entries: AccessEntry[];
  can_manage: boolean;
}

export interface PrincipalRef {
  kind: 'user' | 'group';
  id: string;
}

export interface GrantAccessRequest {
  principals: PrincipalRef[];
  role: Exclude<ShareRole, 'owner'>;
}

export interface UpdateAccessRequest {
  role: Exclude<ShareRole, 'owner'>;
}

/** GET /api/v1/catalog/projects: a server project visible to the caller. */
export interface CatalogProject {
  id: string;
  name: string;
  /** Owner username. */
  owner: string;
  /** "Personal" or a team name. */
  namespace: string;
  latest_version: number | null;
  updated_at: string;
  /** The caller's role on the project. */
  role: ShareRole;
}
