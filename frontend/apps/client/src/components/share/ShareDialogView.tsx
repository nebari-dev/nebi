// Owner: U6 (sharing). The share dialog body (prototype ShareDialog,
// ShareRoleMenu, ShareAdding), without data fetching: the client app
// (ShareDialog.tsx) and the server app wire it to their own APIs. Kept
// identical in apps/server/src/components/share/ShareDialogView.tsx.
import { Avatar, AvatarFallback } from '@nebi/ui/components/ui/avatar';
import { Button } from '@nebi/ui/components/ui/button';
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from '@nebi/ui/components/ui/combobox';
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';
import { Spinner } from '@nebi/ui/components/ui/spinner';
import { cn } from '@nebi/ui/lib/utils';
import { useEffect, useRef, useState } from 'react';
import type { AccessEntry, AccessList, Principal } from '@/types/contracts';
import { ShareRoleMenu } from './ShareRoleMenu';
import {
  type GrantRole,
  isCurrentUser,
  principalDetail,
  principalInitials,
  withoutExisting,
} from './shareModel';

export const PrincipalAvatar = ({
  principal,
  className,
}: {
  principal: Principal;
  className?: string;
}) => (
  <Avatar className={className} aria-hidden>
    <AvatarFallback className="text-[inherit]">
      {principalInitials(principal.name)}
    </AvatarFallback>
  </Avatar>
);

export interface ShareDialogViewProps {
  projectName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Who has access; undefined while loading or after an error. */
  access: AccessList | undefined;
  loading: boolean;
  /** Load error message; the dialog shows it with a Retry button. */
  loadError: string | null;
  onRetry: () => void;
  /** Signed-in username, to mark "(you)". */
  currentUsername?: string | null;
  /** Called with the search text as it is typed ("" on open). */
  onSearchChange: (q: string) => void;
  /** People and teams matching the search, from the server. */
  candidates: Principal[];
  searching: boolean;
  onShare: (principal: Principal, role: GrantRole) => void;
  onChangeRole: (entry: AccessEntry, role: GrantRole) => void;
  onRemove: (entry: AccessEntry) => void;
  /** A change is in flight: the controls are disabled. */
  busy?: boolean;
}

export const ShareDialogView = ({
  projectName,
  open,
  onOpenChange,
  access,
  loading,
  loadError,
  onRetry,
  currentUsername,
  onSearchChange,
  candidates,
  searching,
  onShare,
  onChangeRole,
  onRemove,
  busy = false,
}: ShareDialogViewProps) => {
  const [selected, setSelected] = useState<Principal | null>(null);
  const [role, setRole] = useState<GrantRole>('view');
  const [query, setQuery] = useState('');

  // Every opening starts empty, like the prototype.
  useEffect(() => {
    if (open) {
      setSelected(null);
      setRole('view');
      setQuery('');
    }
  }, [open]);

  const changeQuery = (q: string) => {
    setQuery(q);
    onSearchChange(q);
  };

  const entries = access?.entries ?? [];
  const canManage = access?.can_manage ?? false;

  // Like the prototype, the search field has focus once the dialog opens.
  // It only renders after the access list loads (owners and admins), so
  // focus it then, once per opening.
  const inputRef = useRef<HTMLInputElement>(null);
  const focusedOnOpen = useRef(false);
  useEffect(() => {
    if (!open) {
      focusedOnOpen.current = false;
      return;
    }
    if (!canManage || focusedOnOpen.current) return;
    const frame = requestAnimationFrame(() => {
      focusedOnOpen.current = true;
      inputRef.current?.focus();
    });
    return () => cancelAnimationFrame(frame);
  }, [open, canManage]);
  const items = query.trim() ? withoutExisting(candidates, entries) : [];

  const share = () => {
    if (!selected || busy) return;
    onShare(selected, role);
    setSelected(null);
    changeQuery('');
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[520px]">
        <DialogHeader>
          <DialogTitle>Share {projectName}</DialogTitle>
          <DialogDescription>
            People and teams you add can open this project on the server.
          </DialogDescription>
        </DialogHeader>

        {canManage && (
          <div className="grid gap-1.5">
            <label
              htmlFor="share-with"
              className="font-medium text-foreground text-sm"
            >
              Add people or teams
            </label>
            <div className="flex gap-2">
              <Combobox<Principal>
                items={items}
                filter={null}
                autoHighlight
                value={selected}
                onValueChange={(v) => setSelected(v)}
                inputValue={query}
                onInputValueChange={(v) => {
                  changeQuery(v);
                  if (selected && v !== selected.name) setSelected(null);
                }}
                itemToStringLabel={(p) => p.name}
                isItemEqualToValue={(a, b) =>
                  a.kind === b.kind && a.id === b.id
                }
              >
                <ComboboxInput
                  ref={inputRef}
                  id="share-with"
                  placeholder="Name, email or team"
                  disabled={busy}
                  fieldClassName="flex-1 [&_[data-slot=combobox-trigger]]:hidden"
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && selected) share();
                  }}
                />
                <ComboboxContent>
                  <ComboboxEmpty>
                    {query.trim() && !searching
                      ? 'No one by that name has signed in to this server.'
                      : null}
                  </ComboboxEmpty>
                  <ComboboxList>
                    {(p: Principal) => (
                      <ComboboxItem key={`${p.kind}:${p.id}`} value={p}>
                        <span className="flex min-w-0 items-center gap-2">
                          <PrincipalAvatar
                            principal={p}
                            className="size-5 shrink-0 text-[10px]"
                          />
                          <span className="font-medium text-foreground">
                            {p.name}
                          </span>
                          <span className="truncate text-muted-foreground">
                            {principalDetail(p)}
                          </span>
                        </span>
                      </ComboboxItem>
                    )}
                  </ComboboxList>
                </ComboboxContent>
              </Combobox>
              <ShareRoleMenu
                value={role}
                onChange={setRole}
                variant="outline"
                label="Role for people you add"
                disabled={busy}
              />
              <Button onClick={share} disabled={!selected || busy}>
                Share
              </Button>
            </div>
          </div>
        )}

        <section className="grid gap-2" aria-labelledby="who-has-access">
          <h3
            id="who-has-access"
            className="font-medium text-foreground text-sm"
          >
            Who has access
          </h3>
          {loading && !access ? (
            <div className="flex items-center gap-2 py-2 text-muted-foreground text-sm">
              <Spinner size="xs" />
              Loading…
            </div>
          ) : loadError && !access ? (
            <div className="flex items-center justify-between gap-3 py-1 text-sm">
              <span className="text-destructive-foreground">{loadError}</span>
              <Button variant="outline" size="sm" onClick={onRetry}>
                Retry
              </Button>
            </div>
          ) : (
            <ul className="grid gap-3">
              {entries.map((e) => (
                <li
                  key={`${e.principal.kind}:${e.principal.id}`}
                  className="flex items-center gap-3"
                >
                  <PrincipalAvatar
                    principal={e.principal}
                    className="size-8 text-xs"
                  />
                  <div className="flex min-w-0 flex-1 flex-col">
                    <span className="truncate text-foreground text-sm">
                      {e.principal.name}
                      {isCurrentUser(e.principal, currentUsername) && ' (you)'}
                    </span>
                    <span className="truncate text-muted-foreground text-sm">
                      {principalDetail(e.principal)}
                    </span>
                  </div>
                  {e.role === 'owner' || !canManage ? (
                    <span
                      className={cn(
                        'text-muted-foreground text-sm',
                        e.role !== 'owner' && 'pr-1',
                      )}
                    >
                      {e.role === 'owner'
                        ? 'Owner'
                        : e.role === 'edit'
                          ? 'Can edit'
                          : 'Can view'}
                    </span>
                  ) : (
                    <ShareRoleMenu
                      value={e.role}
                      onChange={(r) => onChangeRole(e, r)}
                      onRemove={() => onRemove(e)}
                      variant="ghost"
                      label={`Access for ${e.principal.name}`}
                      disabled={busy}
                    />
                  )}
                </li>
              ))}
            </ul>
          )}
          {access && !canManage && (
            <p className="text-muted-foreground text-sm">
              Only the project owner or an admin can change who has access.
            </p>
          )}
        </section>

        <DialogFooter>
          <DialogClose render={<Button />}>Done</DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
