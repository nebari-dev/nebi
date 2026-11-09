// Owner: U6 (sharing). Presentation helpers and the toast/undo flow shared
// by the share dialog. This file is kept identical in
// apps/server/src/components/share/shareModel.ts (only the type import
// differs); a later step can move it into packages/ui.
import { toast } from '@nebi/ui/components/ui/toast';
import type {
  AccessEntry,
  AccessList,
  Principal,
  PrincipalRef,
  ShareRole,
} from '@/types/contracts';

export type GrantRole = Exclude<ShareRole, 'owner'>;

/** Roles offered in the role menus, in prototype order. */
export const GRANT_ROLES: GrantRole[] = ['view', 'edit'];

export const ROLE_LABELS: Record<ShareRole, string> = {
  owner: 'Owner',
  edit: 'Can edit',
  view: 'Can view',
};

const ROLE_VERBS: Record<GrantRole, string> = { view: 'view', edit: 'edit' };

/** Second line under a principal: the email, or "Team, N members". */
export const principalDetail = (p: Principal): string =>
  p.kind === 'user'
    ? (p.email ?? '')
    : `Team, ${p.members ?? 0} member${p.members === 1 ? '' : 's'}`;

/** Two-letter initials: "Sam Garcia" -> "SG", "team-jatic" -> "TJ". */
export const principalInitials = (name: string): string => {
  const parts = name
    .trim()
    .split(/[\s._@-]+/)
    .filter(Boolean);
  if (parts.length === 0) return '?';
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
};

export const principalRef = (p: Principal): PrincipalRef => ({
  kind: p.kind,
  id: p.id,
});

/** True when the entry is the signed-in user (rendered as "Name (you)"). */
export const isCurrentUser = (p: Principal, username?: string | null) =>
  p.kind === 'user' && !!username && p.name === username;

/** Server error message of a failed request, if any. */
export const errorMessage = (err: unknown, fallback: string): string => {
  const data = (err as { response?: { data?: { error?: unknown } } })?.response
    ?.data;
  if (data && typeof data.error === 'string' && data.error) return data.error;
  if (err instanceof Error && err.message) return err.message;
  return fallback;
};

/** The calls the dialog makes; each resolves to the new access list. */
export interface ShareBackend {
  grant: (p: PrincipalRef, role: GrantRole) => Promise<AccessList>;
  update: (p: PrincipalRef, role: GrantRole) => Promise<AccessList>;
  revoke: (p: PrincipalRef) => Promise<AccessList>;
}

/**
 * Applies a role change for one principal and shows the prototype toast
 * with an Undo that restores the previous state:
 *   added    -> "Shared <project>"  "<name> can now <view|edit> this project."
 *   changed  -> "Access updated"    "<name> can now <view|edit> <project>."
 *   removed  -> "Access removed"    "<name> can no longer open <project>."
 * `next` null removes access; `prev` null means the principal had none.
 */
export async function applyAccessChange(
  backend: ShareBackend,
  projectName: string,
  principal: Principal,
  next: GrantRole | null,
  prev: GrantRole | null,
): Promise<AccessList | undefined> {
  const ref = principalRef(principal);
  const run = (role: GrantRole | null, had: GrantRole | null) => {
    if (role == null) return backend.revoke(ref);
    if (had == null) return backend.grant(ref, role);
    return backend.update(ref, role);
  };

  let list: AccessList;
  try {
    list = await run(next, prev);
  } catch (err) {
    const title =
      next == null
        ? 'Couldn’t remove access'
        : prev == null
          ? `Couldn’t share ${projectName}`
          : 'Couldn’t update access';
    toast.error(title, {
      description: errorMessage(err, 'The server did not accept the change.'),
    });
    return undefined;
  }

  const undo = {
    children: 'Undo',
    onClick: () => {
      run(prev, next).catch((err) =>
        toast.error('Couldn’t undo', {
          description: errorMessage(
            err,
            'The server did not accept the change.',
          ),
        }),
      );
    },
  };
  if (prev == null && next != null) {
    toast.add({
      type: 'success',
      title: `Shared ${projectName}`,
      description: `${principal.name} can now ${ROLE_VERBS[next]} this project.`,
      actionProps: undo,
    });
  } else if (next != null) {
    toast.add({
      type: 'success',
      title: 'Access updated',
      description: `${principal.name} can now ${ROLE_VERBS[next]} ${projectName}.`,
      actionProps: undo,
    });
  } else {
    toast.add({
      type: 'success',
      title: 'Access removed',
      description: `${principal.name} can no longer open ${projectName}.`,
      actionProps: undo,
    });
  }
  return list;
}

/** Entries a principal search should not offer again. */
export const withoutExisting = (
  candidates: Principal[],
  entries: AccessEntry[],
): Principal[] => {
  const taken = new Set(
    entries.map((e) => `${e.principal.kind}:${e.principal.id}`),
  );
  return candidates.filter((c) => !taken.has(`${c.kind}:${c.id}`));
};
