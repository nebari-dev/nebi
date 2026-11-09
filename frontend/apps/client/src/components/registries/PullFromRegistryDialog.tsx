// Owner: U7 (publish). Used by the projects list ("Pull from registry") and
// the Registries page. Props are a cross-unit contract (CONTRACTS.md); the
// optional `registryId` prop preselects a registry.
//
// The prototype only stubs this flow; the layout follows its "Pull and
// install: local name" dialog: a short form, Cancel, Pull, Pull and install.
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@nebi/ui/components/ui/select';
import { type FormEvent, useEffect, useId, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { clientApiError, clientApiErrorCode } from '@/api/client/base';
import { trackJobToast } from '@/hooks/client/jobs';
import {
  useRegistries,
  useRegistryPull,
  useRegistryRepositories,
  useRegistryTags,
} from '@/hooks/client/publish';
import { AddRegistryDialog } from './AddRegistryDialog';
import { RegistryField } from './RegistryField';
import { SuggestInput } from './SuggestInput';

export interface PullFromRegistryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called with the new local project id once the pull job is submitted. */
  onPulled?: (projectId: string) => void;
  /** Preselect this registry. */
  registryId?: string;
}

type Errors = Partial<
  Record<'registry' | 'repository' | 'tag' | 'local_name' | 'form', string>
>;

const NAME_RE = /^[A-Za-z0-9_.-]+$/;

/** Local name suggested for a repository: its last path segment. */
const nameFromRepository = (repository: string) =>
  repository.trim().replace(/\/+$/, '').split('/').pop() ?? '';

export const PullFromRegistryDialog = ({
  open,
  onOpenChange,
  onPulled,
  registryId: preselected,
}: PullFromRegistryDialogProps) => {
  const navigate = useNavigate();
  const registries = useRegistries();
  const pull = useRegistryPull();
  const [registryId, setRegistryId] = useState('');
  const [repository, setRepository] = useState('');
  const [tag, setTag] = useState('');
  const [localName, setLocalName] = useState('');
  const [nameTouched, setNameTouched] = useState(false);
  const [errors, setErrors] = useState<Errors>({});
  const [installing, setInstalling] = useState(false);
  const [addOpen, setAddOpen] = useState(false);
  const registryLabelId = useId();

  const list = registries.data ?? [];
  const repos = useRegistryRepositories(open ? registryId : '', '');
  const repoTrimmed = repository.trim().replace(/^\/+|\/+$/g, '');
  const tags = useRegistryTags(open ? registryId : '', repoTrimmed);

  useEffect(() => {
    if (open) {
      setRepository('');
      setTag('');
      setLocalName('');
      setNameTouched(false);
      setErrors({});
    }
  }, [open]);

  // Pick the preselected, else the default, else the first registry.
  useEffect(() => {
    if (!open || list.length === 0) return;
    if (registryId && list.some((r) => r.id === registryId)) return;
    const pick =
      list.find((r) => r.id === preselected) ??
      list.find((r) => r.is_default) ??
      list[0];
    setRegistryId(pick.id);
  }, [open, list, preselected, registryId]);

  const chooseRepository = (value: string) => {
    setRepository(value);
    setErrors((e) => ({ ...e, repository: undefined, form: undefined }));
    if (!nameTouched) setLocalName(nameFromRepository(value));
  };

  const submit = async (install: boolean) => {
    const name = localName.trim() || nameFromRepository(repoTrimmed);
    const found: Errors = {};
    if (!registryId) found.registry = 'Choose a registry.';
    if (!repoTrimmed) found.repository = 'Choose a repository.';
    if (!tag.trim()) found.tag = 'Choose a tag.';
    if (!name) found.local_name = 'Give the project a name.';
    else if (!NAME_RE.test(name))
      found.local_name = 'Use letters, numbers, hyphens, underscores or dots.';
    setErrors(found);
    if (Object.keys(found).length > 0) return;
    setInstalling(install);
    try {
      const res = await pull.mutateAsync({
        registryId,
        repository: repoTrimmed,
        tag: tag.trim(),
        local_name: name,
        install,
      });
      trackJobToast(res.job, {
        title: name,
        successTitle: `${name} pulled`,
        successDescription: install
          ? 'Version 1 is on this machine. Installing it now.'
          : 'Version 1 is on this machine. Install it when you need it.',
        errorTitle: `Couldn’t pull ${name}`,
        cancellable: true,
        onViewJob: (job) => navigate(`/jobs/${job.id}`),
      });
      onPulled?.(res.project_id);
      onOpenChange(false);
    } catch (err) {
      const apiErr = clientApiError(err);
      const message = apiErr?.error ?? 'Couldn’t start the pull.';
      if (clientApiErrorCode(err) === 'name_taken') {
        setErrors({ local_name: message });
      } else if (
        apiErr?.field === 'repository' ||
        apiErr?.field === 'tag' ||
        apiErr?.field === 'local_name'
      ) {
        setErrors({ [apiErr.field]: message });
      } else {
        setErrors({ form: message });
      }
    }
  };

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    void submit(false);
  };

  const noRegistries = registries.isSuccess && list.length === 0;
  const repoError = repos.isError
    ? 'Couldn’t list repositories on this registry. Type the repository instead.'
    : undefined;

  return (
    <>
      <Dialog open={open && !addOpen} onOpenChange={onOpenChange}>
        <DialogContent className="max-w-[520px] overflow-auto">
          <form onSubmit={onSubmit} className="grid gap-4" noValidate>
            <DialogHeader>
              <DialogTitle>Pull from registry</DialogTitle>
              <DialogDescription>
                Bring a project from a registry onto this machine. It becomes
                version 1 of a new project.
              </DialogDescription>
            </DialogHeader>
            {noRegistries ? (
              <section className="flex flex-col items-start gap-2">
                <h3 className="font-medium text-foreground text-sm">
                  No registries yet
                </h3>
                <p className="text-muted-foreground text-sm">
                  Add the registry the project is published to, then pull it.
                </p>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => setAddOpen(true)}
                >
                  Add a registry
                </Button>
              </section>
            ) : (
              <>
                <div className="grid gap-1">
                  <span
                    id={registryLabelId}
                    className="font-medium text-foreground text-sm"
                  >
                    Registry
                  </span>
                  <Select
                    value={registryId || null}
                    onValueChange={(v) => {
                      setRegistryId((v as string) ?? '');
                      setRepository('');
                      setTag('');
                    }}
                    items={list.map((r) => ({ value: r.id, label: r.name }))}
                  >
                    <SelectTrigger aria-labelledby={registryLabelId}>
                      <SelectValue placeholder="Choose a registry" />
                    </SelectTrigger>
                    <SelectContent>
                      {list.map((r) => (
                        <SelectItem key={r.id} value={r.id}>
                          {r.name}
                          <span className="text-muted-foreground text-xs">
                            {r.url}
                            {r.namespace ? `/${r.namespace}` : ''}
                          </span>
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {errors.registry && (
                    <p className="text-destructive-foreground text-xs">
                      {errors.registry}
                    </p>
                  )}
                </div>
                <RegistryField
                  label="Repository"
                  error={errors.repository}
                  description={repoError}
                >
                  <SuggestInput
                    value={repository}
                    onValueChange={chooseRepository}
                    suggestions={repos.data ?? []}
                    placeholder="e.g., nebari/project_1"
                    emptyText={
                      repos.isFetching ? 'Loading…' : 'No repositories found'
                    }
                  />
                </RegistryField>
                <RegistryField label="Tag" error={errors.tag}>
                  <SuggestInput
                    value={tag}
                    onValueChange={(v) => {
                      setTag(v);
                      setErrors((e) => ({ ...e, tag: undefined }));
                    }}
                    suggestions={tags.data ?? []}
                    placeholder="e.g., 5"
                    emptyText={tags.isFetching ? 'Loading…' : 'No tags found'}
                  />
                </RegistryField>
                <RegistryField
                  label="Local name"
                  description="The project’s name on this machine."
                  error={errors.local_name}
                >
                  <Input
                    value={localName}
                    onChange={(e) => {
                      setLocalName(e.target.value);
                      setNameTouched(true);
                      setErrors((er) => ({ ...er, local_name: undefined }));
                    }}
                    spellCheck={false}
                  />
                </RegistryField>
                {errors.form && (
                  <p
                    role="alert"
                    className="text-destructive-foreground text-sm"
                  >
                    {errors.form}
                  </p>
                )}
              </>
            )}
            <DialogFooter>
              <DialogClose render={<Button variant="secondary" />}>
                Cancel
              </DialogClose>
              {!noRegistries && (
                <>
                  <Button
                    variant="secondary"
                    render={<button type="submit" />}
                    loading={pull.isPending && !installing}
                    loadingText="Pulling…"
                    disabled={pull.isPending}
                  >
                    Pull
                  </Button>
                  <Button
                    loading={pull.isPending && installing}
                    loadingText="Pulling…"
                    disabled={pull.isPending}
                    onClick={() => void submit(true)}
                  >
                    Pull and install
                  </Button>
                </>
              )}
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <AddRegistryDialog
        open={open && addOpen}
        onOpenChange={setAddOpen}
        onAdded={(r) => setRegistryId(r.id)}
      />
    </>
  );
};
