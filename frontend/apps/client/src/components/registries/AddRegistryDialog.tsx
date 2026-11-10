// Owner: U7 (publish). Used by the Registries page and the projects list
// ("Add a registry"). Props are a cross-unit contract (CONTRACTS.md); the
// optional `registry` prop (edit mode) is an addition used by the
// Registries page.
import { Button } from '@nebi/ui/components/ui/button';
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';
import { Input } from '@nebi/ui/components/ui/input';
import { Switch } from '@nebi/ui/components/ui/switch';
import { Tabs, TabsList, TabsTab } from '@nebi/ui/components/ui/tabs';
import { toast } from '@nebi/ui/components/ui/toast';
import { type FormEvent, useEffect, useId, useState } from 'react';
import { clientApiError } from '@/api/client/base';
import { useAddRegistry, useUpdateRegistry } from '@/hooks/client/publish';
import type { RegistryItem } from '@/types/contracts';
import { RegistryField } from './RegistryField';

export interface AddRegistryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onAdded?: (registry: RegistryItem) => void;
  /** Edit this registry instead of adding one. */
  registry?: RegistryItem | null;
}

type Auth = 'none' | 'password';
type FieldName = 'name' | 'url' | 'namespace' | 'username' | 'password';
type Errors = Partial<Record<FieldName | 'form', string>>;

const URL_RE = /^[a-z0-9]([a-z0-9.-]*[a-z0-9])?(:\d+)?(\/[a-z0-9._/-]+)?$/i;
const NAMESPACE_RE = /^[a-z0-9]([a-z0-9._/-]*[a-z0-9])?$/;

interface FormState {
  name: string;
  url: string;
  namespace: string;
  auth: Auth;
  username: string;
  password: string;
  isDefault: boolean;
}

const initialState = (registry?: RegistryItem | null): FormState => ({
  name: registry?.name ?? '',
  url: registry?.url ?? '',
  namespace: registry?.namespace ?? '',
  auth: registry?.has_credentials ? 'password' : 'none',
  username: registry?.username ?? '',
  password: '',
  isDefault: registry?.is_default ?? false,
});

function validate(form: FormState, editing: RegistryItem | null): Errors {
  const errors: Errors = {};
  const url = form.url
    .trim()
    .replace(/^https?:\/\//, '')
    .replace(/\/$/, '');
  if (!form.name.trim()) errors.name = 'Enter a registry name.';
  if (!url) errors.url = 'Enter the registry’s URL.';
  else if (!URL_RE.test(url))
    errors.url = 'That doesn’t look like a registry URL. Try ghcr.io.';
  const ns = form.namespace.trim().replace(/^\/|\/$/g, '');
  if (ns && !NAMESPACE_RE.test(ns))
    errors.namespace =
      'Use lowercase letters, numbers, dots, hyphens, underscores or slashes.';
  if (form.auth === 'password') {
    if (!form.username.trim())
      errors.username = 'Enter a username, or choose None.';
    const keepsPassword = !!editing?.has_credentials;
    if (!form.password && !keepsPassword)
      errors.password = 'Enter a password or token, or choose None.';
  }
  return errors;
}

export const AddRegistryDialog = ({
  open,
  onOpenChange,
  onAdded,
  registry = null,
}: AddRegistryDialogProps) => {
  const editing = registry;
  const [form, setForm] = useState<FormState>(() => initialState(editing));
  const [errors, setErrors] = useState<Errors>({});
  const add = useAddRegistry();
  const update = useUpdateRegistry();
  const authLabelId = useId();
  const defaultLabelId = useId();
  const saving = add.isPending || update.isPending;

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
    setErrors((e) => ({ ...e, [key]: undefined, form: undefined }));
  };

  // Start from the registry (or a blank form) every time the dialog opens.
  useEffect(() => {
    if (open) {
      setForm(initialState(editing));
      setErrors({});
    }
  }, [open, editing]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const found = validate(form, editing);
    setErrors(found);
    if (Object.keys(found).length > 0) return;
    const withAuth = form.auth === 'password';
    const base = {
      name: form.name.trim(),
      url: form.url.trim(),
      namespace: form.namespace.trim(),
      username: withAuth ? form.username.trim() : '',
      is_default: form.isDefault,
    };
    try {
      let saved: RegistryItem;
      if (editing) {
        saved = await update.mutateAsync({
          id: editing.id,
          ...base,
          // Blank keeps the stored password; None removes it.
          password: withAuth ? form.password || undefined : '',
        });
        toast.add({ title: 'Registry updated', type: 'success' });
      } else {
        saved = await add.mutateAsync({
          ...base,
          password: withAuth ? form.password : '',
        });
        toast.add({
          title: `${saved.name} added`,
          description:
            'You can publish versions to it and pull projects from it.',
          type: 'success',
        });
      }
      onAdded?.(saved);
      onOpenChange(false);
    } catch (err) {
      const apiErr = clientApiError(err);
      const field = apiErr?.field as FieldName | undefined;
      const message = apiErr?.error ?? 'Couldn’t save the registry.';
      if (
        field &&
        ['name', 'url', 'namespace', 'username', 'password'].includes(field)
      ) {
        setErrors({ [field]: message });
      } else {
        setErrors({ form: message });
      }
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[520px] overflow-auto">
        <form onSubmit={submit} className="grid gap-4" noValidate>
          <DialogHeader>
            <DialogTitle>
              {editing ? 'Edit registry' : 'Add a registry'}
            </DialogTitle>
            <DialogDescription>
              Publish versions to an OCI registry and pull projects from it.
              Passwords are kept in your system keychain.
            </DialogDescription>
          </DialogHeader>
          <RegistryField label="Name" error={errors.name}>
            <Input
              value={form.name}
              onChange={(e) => set('name', e.target.value)}
              placeholder="e.g., GitHub Container Registry"
              autoFocus={!editing}
              spellCheck={false}
            />
          </RegistryField>
          <RegistryField label="Registry URL" error={errors.url}>
            <Input
              value={form.url}
              onChange={(e) => set('url', e.target.value)}
              placeholder="ghcr.io"
              spellCheck={false}
            />
          </RegistryField>
          <RegistryField
            label="Namespace (optional)"
            description="The organization or user your repositories live under, e.g. example-org."
            error={errors.namespace}
          >
            <Input
              value={form.namespace}
              onChange={(e) => set('namespace', e.target.value)}
              placeholder="example-org"
              spellCheck={false}
            />
          </RegistryField>
          <div className="grid gap-1">
            <span
              id={authLabelId}
              className="font-medium text-foreground text-sm"
            >
              Authentication
            </span>
            <Tabs
              value={form.auth}
              onValueChange={(v) => set('auth', v as Auth)}
            >
              <TabsList
                variant="toggle"
                aria-labelledby={authLabelId}
                className="w-full"
              >
                <TabsTab value="none" className="flex-1">
                  None
                </TabsTab>
                <TabsTab value="password" className="flex-1">
                  Username and password
                </TabsTab>
              </TabsList>
            </Tabs>
          </div>
          {form.auth === 'password' && (
            // Side by side so the dialog fits a laptop screen without
            // scrolling to the footer.
            <div className="grid grid-cols-2 items-start gap-3">
              <RegistryField label="Username" error={errors.username}>
                <Input
                  value={form.username}
                  onChange={(e) => set('username', e.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                />
              </RegistryField>
              <RegistryField
                label="Password or token"
                description={
                  editing?.has_credentials
                    ? 'Leave blank to keep the saved password.'
                    : undefined
                }
                error={errors.password}
              >
                <Input
                  type="password"
                  value={form.password}
                  onChange={(e) => set('password', e.target.value)}
                  autoComplete="off"
                />
              </RegistryField>
            </div>
          )}
          <div className="flex items-center justify-between gap-4">
            <div className="flex flex-col gap-0.5">
              <span
                id={defaultLabelId}
                className="font-medium text-foreground text-sm"
              >
                Default registry
              </span>
              <span className="text-muted-foreground text-xs">
                Used when no registry is chosen, for example by the CLI.
              </span>
            </div>
            <Switch
              aria-labelledby={defaultLabelId}
              checked={form.isDefault}
              onCheckedChange={(checked) => set('isDefault', checked)}
            />
          </div>
          {errors.form && (
            <p role="alert" className="text-destructive-foreground text-sm">
              {errors.form}
            </p>
          )}
          <DialogFooter>
            <DialogClose render={<Button variant="secondary" />}>
              Cancel
            </DialogClose>
            <Button
              render={<button type="submit" />}
              loading={saving}
              loadingText={editing ? 'Saving…' : 'Adding…'}
            >
              {editing ? 'Save changes' : 'Add registry'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
};
