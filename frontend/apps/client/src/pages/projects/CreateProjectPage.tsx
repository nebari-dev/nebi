// Owner: U1 (projects). New project (prototype CreateProject,
// CreateProjectToml, DiscardNewProject): name and folder, the environment
// from the GUI form or the pixi.toml editor, the description and tags of
// version 1, then Create or Create and install.
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@nebi/ui/components/ui/breadcrumb';
import { Button } from '@nebi/ui/components/ui/button';
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';
import { Input } from '@nebi/ui/components/ui/input';
import { toast } from '@nebi/ui/components/ui/toast';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@nebi/ui/components/ui/tooltip';
import { cn } from '@nebi/ui/lib/utils';
import { FolderOpen } from 'lucide-react';
import { type ReactNode, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { clientApiError } from '@/api/client/base';
import { ChipsField } from '@/components/manifest/ChipsField';
import {
  FORM_COLUMN,
  FormRow,
  FormSection,
} from '@/components/manifest/FormLayout';
import {
  ManifestEditor,
  type ManifestEditorErrors,
} from '@/components/manifest/ManifestEditor';
import { ManifestModeSwitch } from '@/components/manifest/ManifestModeSwitch';
import {
  DEFAULT_CHANNEL,
  formToToml,
  type ManifestEditorValue,
  sameValue,
  setTomlName,
} from '@/components/manifest/manifestValue';
import {
  machinePlatform,
  NAME_MESSAGES,
  projectNameError,
  tagError,
} from '@/components/manifest/rules';
import { trackJobToast } from '@/hooks/client/jobs';
import {
  useCapabilities,
  useCreateProject,
  usePickFolder,
  useProjects,
} from '@/hooks/client/projects';
import type { CreateProjectRequest, ManifestForm } from '@/types/contracts';

const DEFAULT_DESCRIPTION = 'Initial project creation';
const DEFAULT_TAGS = ['latest'];
const DEFAULT_REQUESTED = [{ name: 'python', constraint: '>=3.11' }];

type FieldErrors = ManifestEditorErrors & {
  path?: string;
  description?: string;
  tags?: string;
};

const PageHeader = ({ action }: { action?: ReactNode }) => (
  <div className="flex flex-wrap items-start justify-between gap-4">
    <h1 className="font-bold text-3xl text-foreground">New project</h1>
    {action}
  </div>
);

const initialFormFor = (machine: string): ManifestForm => ({
  name: '',
  channels: [DEFAULT_CHANNEL],
  platforms: [machine],
  requested: DEFAULT_REQUESTED,
});

export const CreateProjectPage = () => {
  const capabilities = useCapabilities().data;
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const projects = useProjects();
  const create = useCreateProject();
  const pickFolder = usePickFolder();
  const machine = machinePlatform(capabilities?.platform);

  const initialForm = useMemo(() => initialFormFor(machine), [machine]);
  const startInToml = params.get('mode') === 'toml';
  const [name, setName] = useState('');
  const [path, setPath] = useState('');
  const [value, setValue] = useState<ManifestEditorValue>(() => ({
    mode: startInToml ? 'toml' : 'form',
    form: initialForm,
    pixiToml: startInToml ? formToToml(initialForm) : '',
  }));
  const [description, setDescription] = useState(DEFAULT_DESCRIPTION);
  const [tags, setTags] = useState<string[]>(DEFAULT_TAGS);
  const [submitted, setSubmitted] = useState(false);
  const [serverErrors, setServerErrors] = useState<FieldErrors>({});
  const [discardTo, setDiscardTo] = useState<string | null>(null);
  const [creating, setCreating] = useState<'create' | 'install' | null>(null);
  const [done, setDone] = useState(false);
  const nameRef = useRef<HTMLInputElement>(null);
  const descriptionRef = useRef<HTMLInputElement>(null);

  // The page opens before GET /client/capabilities answers (the user agent
  // guesses the platform meanwhile); once the host's platform is known,
  // swap it in unless the user already changed the platforms.
  const shownMachine = useRef(machine);
  useEffect(() => {
    const prev = shownMachine.current;
    if (prev === machine) return;
    shownMachine.current = machine;
    setValue((v) => {
      if (v.mode === 'form' && sameValue(v.form.platforms, [prev])) {
        return { ...v, form: { ...v.form, platforms: [machine] } };
      }
      const before = { ...initialFormFor(prev), name: v.form.name };
      if (v.mode === 'toml' && v.pixiToml === formToToml(before)) {
        const form = { ...before, platforms: [machine] };
        return { ...v, form, pixiToml: formToToml(form) };
      }
      return v;
    });
  }, [machine]);

  const trimmedName = name.trim();
  const existingNames = useMemo(
    () => (projects.data ?? []).map((p) => p.name),
    [projects.data],
  );

  // Client-side checks (the server repeats them).
  const liveNameError = trimmedName
    ? projectNameError(trimmedName, existingNames)
    : submitted
      ? NAME_MESSAGES.required
      : null;
  const errors: FieldErrors = {
    name: serverErrors.name ?? liveNameError ?? undefined,
    path: serverErrors.path,
    platforms:
      serverErrors.platforms ??
      (value.mode === 'form' && submitted && value.form.platforms.length === 0
        ? 'Add at least one platform.'
        : undefined),
    channels: serverErrors.channels,
    requested: serverErrors.requested,
    manifest: serverErrors.manifest,
    description:
      serverErrors.description ??
      (submitted && !description.trim() ? 'Enter a description.' : undefined),
    tags: serverErrors.tags,
  };
  const badTags = tags.some((t) => tagError(t));

  const dirty =
    !done &&
    (!!trimmedName ||
      !!path.trim() ||
      !sameValue(tags, DEFAULT_TAGS) ||
      description !== DEFAULT_DESCRIPTION ||
      (value.mode === 'form'
        ? !sameValue(value.form.platforms, initialForm.platforms) ||
          !sameValue(value.form.channels, initialForm.channels) ||
          !sameValue(value.form.requested, initialForm.requested)
        : value.pixiToml.trim() !==
          formToToml({ ...initialForm, name: trimmedName }).trim()));

  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [dirty]);

  const leave = (to: string) => (dirty ? setDiscardTo(to) : navigate(to));
  const clearServerError = (...keys: (keyof FieldErrors)[]) =>
    setServerErrors((e) => {
      const next = { ...e };
      for (const k of keys) delete next[k];
      return next;
    });

  const changeValue = (next: ManifestEditorValue) => {
    setValue(next);
    clearServerError('platforms', 'channels', 'requested', 'manifest');
  };
  const changeName = (next: string) => {
    setName(next);
    clearServerError('name');
    setValue((v) => ({
      ...v,
      form: { ...v.form, name: next.trim() },
      pixiToml:
        v.mode === 'toml' ? setTomlName(v.pixiToml, next.trim()) : v.pixiToml,
    }));
  };
  const switchMode = (next: ManifestEditorValue) => {
    const nextParams = new URLSearchParams(params);
    if (next.mode === 'toml') nextParams.set('mode', 'toml');
    else nextParams.delete('mode');
    setParams(nextParams, { replace: true });
    if (!trimmedName && next.form.name) setName(next.form.name);
    setSubmitted(false);
  };

  const applyServerError = (err: unknown) => {
    const e = clientApiError(err);
    if (e?.code === 'name_taken') {
      setServerErrors({ name: e.error });
      nameRef.current?.focus();
      return;
    }
    if (e?.field) {
      const field =
        value.mode === 'toml' &&
        ['platforms', 'channels', 'packages', 'requested'].includes(e.field)
          ? 'manifest'
          : e.field === 'packages'
            ? 'requested'
            : e.field;
      if (
        [
          'name',
          'path',
          'platforms',
          'channels',
          'requested',
          'manifest',
          'description',
          'tags',
        ].includes(field)
      ) {
        setServerErrors({ [field]: e.error });
        if (field === 'name') nameRef.current?.focus();
        if (field === 'description') descriptionRef.current?.focus();
        return;
      }
    }
    toast.add({
      type: 'error',
      title: 'Couldn’t create the project',
      description: e?.error,
    });
  };

  const submit = (install: boolean) => {
    setSubmitted(true);
    const nameProblem = trimmedName
      ? projectNameError(trimmedName, existingNames)
      : NAME_MESSAGES.required;
    const noPlatforms =
      value.mode === 'form' && value.form.platforms.length === 0;
    if (nameProblem || noPlatforms || !description.trim() || badTags) {
      if (nameProblem) nameRef.current?.focus();
      else if (!description.trim()) descriptionRef.current?.focus();
      return;
    }
    const req: CreateProjectRequest = {
      name: trimmedName,
      path: path.trim() || undefined,
      description: description.trim(),
      tags,
      install,
      ...(value.mode === 'form'
        ? { form: { ...value.form, name: trimmedName } }
        : { pixi_toml: value.pixiToml }),
    };
    setCreating(install ? 'install' : 'create');
    create.mutate(req, {
      onSuccess: (res) => {
        setDone(true);
        trackJobToast(res.job, {
          title: install
            ? `Creating and installing ${res.project.name}`
            : `Creating ${res.project.name}`,
          successTitle: `${res.project.name} created`,
          successDescription: install
            ? 'Version 1 is saved and installed. Ready to use on this machine.'
            : 'Version 1 is saved on this machine. Install it when you need it.',
          errorTitle: `${res.project.name} couldn’t be created`,
          onViewJob: (job) => navigate(`/jobs/${job.id}`),
        });
        navigate(`/projects/${res.project.id}?v=1`, { replace: true });
      },
      onError: applyServerError,
      onSettled: () => setCreating(null),
    });
  };

  return (
    <main className="flex flex-1 flex-col px-9 pt-9 max-[640px]:px-4">
      <form
        className="flex flex-1 flex-col"
        onSubmit={(e) => e.preventDefault()}
        noValidate
      >
        <div className={cn(FORM_COLUMN, 'flex flex-col gap-1.5 pb-6')}>
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem>
                <BreadcrumbLink
                  render={<Link to="/" />}
                  onClick={(e) => {
                    e.preventDefault();
                    leave('/');
                  }}
                >
                  Projects
                </BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>New project</BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
          <PageHeader
            action={
              <ManifestModeSwitch
                value={value}
                onChange={changeValue}
                onSwitched={switchMode}
                disabled={!!creating}
              />
            }
          />
        </div>

        <div className={FORM_COLUMN}>
          <FormSection
            id="project"
            title="Project"
            intro="Its name, and where its folder goes on this machine."
          >
            <FormRow label="Project name" error={errors.name}>
              <Input
                ref={nameRef}
                value={name}
                onChange={(e) => changeName(e.target.value)}
                placeholder="e.g., my-data-project"
                autoComplete="off"
                spellCheck={false}
              />
            </FormRow>
            <FormRow
              label="Path (optional)"
              description="Where the project folder is created. Leave blank to use Nebi’s default location."
              error={errors.path}
            >
              <div className="relative">
                <Input
                  value={path}
                  onChange={(e) => {
                    setPath(e.target.value);
                    clearServerError('path');
                  }}
                  placeholder="e.g., /home/user/projects/my-data-project"
                  autoComplete="off"
                  spellCheck={false}
                  className={capabilities?.native_folder_picker ? 'pr-10' : ''}
                />
                {capabilities?.native_folder_picker && (
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon-xs"
                          className="absolute top-1/2 right-1.5 -translate-y-1/2 text-muted-foreground"
                          aria-label="Choose a folder"
                          loading={pickFolder.isPending}
                          onClick={() =>
                            pickFolder.mutate(
                              {
                                title: 'Choose a folder',
                                default_path:
                                  path.trim() ||
                                  capabilities.default_projects_dir,
                              },
                              {
                                onSuccess: (res) => {
                                  if (!res.cancelled && res.path) {
                                    setPath(res.path);
                                    clearServerError('path');
                                  }
                                },
                                onError: (err) =>
                                  toast.add({
                                    type: 'error',
                                    title: 'Couldn’t open the folder picker',
                                    description: clientApiError(err)?.error,
                                  }),
                              },
                            )
                          }
                        />
                      }
                    >
                      <FolderOpen className="size-[18px]" />
                    </TooltipTrigger>
                    <TooltipContent>Choose a folder</TooltipContent>
                  </Tooltip>
                )}
              </div>
            </FormRow>
          </FormSection>

          <ManifestEditor
            value={value}
            onChange={changeValue}
            errors={errors}
            machine={machine}
            disabled={!!creating}
            hideModeSwitch
          />

          <FormSection
            id="about"
            title="About version 1"
            intro="Shown in the version list and on the version’s page."
          >
            <FormRow label="Description" error={errors.description}>
              <Input
                ref={descriptionRef}
                value={description}
                onChange={(e) => {
                  setDescription(e.target.value);
                  clearServerError('description');
                }}
                autoComplete="off"
              />
            </FormRow>
            <ChipsField
              label="Tags (optional)"
              value={tags}
              onChange={(next) => {
                setTags(next);
                clearServerError('tags');
              }}
              suggestions={DEFAULT_TAGS}
              placeholder="Add a tag"
              keepPlaceholder
              description="A tag stays with this version until a newer version uses the same tag. latest does this automatically."
              invalid={tagError}
              error={errors.tags}
            />
          </FormSection>
        </div>

        <footer className="sticky bottom-0 z-10 -mx-9 mt-auto bg-canvas px-9 max-[640px]:-mx-4 max-[640px]:px-4">
          <div
            className={cn(
              FORM_COLUMN,
              'flex items-center justify-between gap-4 border-border border-t py-3',
            )}
          >
            <Button type="button" variant="ghost" onClick={() => leave('/')}>
              Cancel
            </Button>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="secondary"
                loading={creating === 'create'}
                disabled={!!creating}
                onClick={() => submit(false)}
              >
                Create
              </Button>
              <Button
                type="button"
                loading={creating === 'install'}
                disabled={!!creating}
                onClick={() => submit(true)}
              >
                Create and install
              </Button>
            </div>
          </div>
        </footer>
      </form>

      <Dialog
        open={discardTo != null}
        onOpenChange={(open) => !open && setDiscardTo(null)}
      >
        <DialogContent className="max-w-[520px]" role="alertdialog">
          <DialogHeader>
            <DialogTitle>Discard new project?</DialogTitle>
          </DialogHeader>
          <p className="text-muted-foreground-strong text-sm">
            Any filled data will be discarded.
          </p>
          <DialogFooter>
            <DialogClose render={<Button variant="secondary" />}>
              Go back
            </DialogClose>
            <Button
              variant="destructive"
              onClick={() => {
                const to = discardTo;
                setDone(true);
                setDiscardTo(null);
                if (to) navigate(to);
              }}
            >
              Discard
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </main>
  );
};
