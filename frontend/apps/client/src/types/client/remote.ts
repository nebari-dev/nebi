// Owner: the remote unit. Unit-specific frontend types; the API contract types
// live in types/contracts.ts (frozen).
import type { ServerProjectItem } from '@/types/contracts';

/**
 * Optional body of POST /client/projects/:id/push. up_to limits the push to
 * versions numbered up to N ("Push to server" on version N); without it
 * every version not yet on the server is pushed.
 */
export interface PushRequest {
  up_to?: number;
}

/** Variables of usePushProject: a project id, or a project id and a version. */
export type PushVariables = string | { projectId: string; upTo?: number };

/** Dialogs the Server page can open (also via ?dialog=…&project=…). */
export type ServerDialog =
  | { kind: 'connect' }
  | { kind: 'manage' }
  | { kind: 'pull'; project: ServerProjectItem; andInstall: boolean }
  | { kind: 'share'; project: ServerProjectItem };

/** A pull in progress, shown as a loading row action on the Server page. */
export interface PullingState {
  serverProjectId: string;
  localProjectId: string | null;
  installing: boolean;
}
