// Owner: U3 (newversion). "New version of <project>" (prototype screens
// CreateNewVersion, CreateNewVersionToml, JustCreated):
// /projects/:id/new-version?from=N[&mode=toml].
//
// The page starts from version N's pixi.toml (latest when `from` is
// missing), edits it in the GUI form or the pixi.toml editor (the shared
// ManifestEditor), shows the summary of changes against version N, and
// creates the version as a create_version job. "Create and install" also
// installs the new version once it exists. Leaving with unsaved changes
// asks "Discard new version?".
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
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@nebi/ui/components/ui/field';
import { Input } from '@nebi/ui/components/ui/input';
import { toast } from '@nebi/ui/components/ui/toast';
import { cn } from '@nebi/ui/lib/utils';
import { Loader2 } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import {
  Link,
  useNavigate,
  useParams,
  useSearchParams,
} from 'react-router-dom';
import { clientApiError } from '@/api/client/base';
import { detailApi } from '@/api/client/detail';
import { remoteApi } from '@/api/client/remote';
import {
  ManifestEditor,
  type ManifestEditorErrors,
} from '@/components/manifest/ManifestEditor';
import type { ManifestEditorValue } from '@/components/manifest/manifestValue';
import { ChangesSummary } from '@/components/newversion/ChangesSummary';
import {
  sameForm,
  suggestDescription,
  summarizeRequested,
  tagError,
  tagsForRequest,
} from '@/components/newversion/changes';
import { DiscardNewVersionDialog } from '@/components/newversion/DiscardNewVersionDialog';
import { FormSection, PAGE_COLUMN } from '@/components/newversion/FormSection';
import { TagsInput } from '@/components/newversion/TagsInput';
import { useUnsavedChangesGuard } from '@/components/newversion/useUnsavedChangesGuard';
import { capitalize, versionList } from '@/components/server/format';
import { trackJobToast } from '@/hooks/client/jobs';
import {
  useChangesSummary,
  useCreateVersion,
  useVersionDraft,
} from '@/hooks/client/newversion';
import { useProject } from '@/hooks/client/projects';
import type {
  PreviewChangesRequest,
  VersionDraft,
} from '@/types/client/newversion';
import type { CreateVersionRequest } from '@/types/contracts';

/** Debounces a JSON-serializable value (stable by content). */
function useDebouncedValue<T>(value: T, ms: number): T {
  const key = JSON.stringify(value);
  const [debounced, setDebounced] = useState(value);
  // biome-ignore lint/correctness/useExhaustiveDependencies: keyed by content
  useEffect(() => {
    const t = window.setTimeout(() => setDebounced(value), ms);
    return () => window.clearTimeout(t);
  }, [key, ms]);
  return debounced;
}

/** The manifest part of a request: pixi.toml, or the form (applied onto
 * the edited pixi.toml when the user changed it before switching). */
function manifestPart(
  value: ManifestEditorValue,
  draft: VersionDraft,
): Pick<CreateVersionRequest, 'pixi_toml' | 'form'> {
  if (value.mode === 'toml') return { pixi_toml: value.pixiToml };
  const edited =
    value.pixiToml.trim() !== '' &&
    value.pixiToml.trim() !== draft.pixi_toml.trim();
  return edited
    ? { form: value.form, pixi_toml: value.pixiToml }
    : { form: value.form };
}

/** Server field names to ManifestEditor error keys. */
const EDITOR_FIELDS: Record<string, keyof ManifestEditorErrors> = {
  manifest: 'manifest',
  platforms: 'platforms',
  channels: 'channels',
  packages: 'requested',
  name: 'name',
};

function NotFound() {
  return (
    <main className="flex flex-col gap-3 p-9">
      <h1 className="font-bold text-3xl text-foreground">Project not found</h1>
      <Link to="/" className="text-sm underline underline-offset-4">
        Back to projects
      </Link>
    </main>
  );
}

export const NewVersionPage = () => {
  const { id = '' } = useParams();
  const [params] = useSearchParams();
  const fromParam = Number(params.get('from'));
  const from =
    Number.isInteger(fromParam) && fromParam > 0 ? fromParam : undefined;
  const draft = useVersionDraft(id, from);

  if (draft.isPending) {
    return (
      <main className="flex flex-1 flex-col px-9 pt-9" aria-busy>
        <div className={cn(PAGE_COLUMN, 'flex flex-col gap-3')}>
          <p className="flex items-center gap-2 text-muted-foreground text-sm">
            <Loader2 className="size-4 animate-spin" aria-hidden />
            Loading…
          </p>
        </div>
      </main>
    );
  }
  if (draft.isError || !draft.data) return <NotFound />;
  return (
    <NewVersionForm
      key={`${id}:${draft.data.base_version}`}
      projectId={id}
      draft={draft.data}
    />
  );
};

function NewVersionForm({
  projectId,
  draft,
}: {
  projectId: string;
  draft: VersionDraft;
}) {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const base = draft.base_version;
  const name = draft.project_name;
  const detailPath = `/projects/${projectId}?v=${base}`;

  const [value, setValue] = useState<ManifestEditorValue>(() => ({
    mode: params.get('mode') === 'toml' ? 'toml' : 'form',
    form: draft.form,
    pixiToml: draft.pixi_toml,
  }));
  const [tags, setTags] = useState<string[]>(['latest']);
  const [description, setDescription] = useState<string | null>(null);
  const [submitted, setSubmitted] = useState(false);
  const [serverErrors, setServerErrors] = useState<Record<string, string>>({});
  const [created, setCreated] = useState(false);
  /** Which button submitted: false = Create version, true = and install. */
  const [submitting, setSubmitting] = useState<boolean | null>(null);
  const descriptionRef = useRef<HTMLInputElement>(null);
  const create = useCreateVersion(projectId);
  const { data: project } = useProject(projectId);
  const linked = !!project?.remotes.some((r) => r.kind === 'server');

  // Keep ?mode=toml in the URL in step with the editor mode.
  useEffect(() => {
    const want = value.mode === 'toml' ? 'toml' : null;
    if (params.get('mode') === want) return;
    const next = new URLSearchParams(params);
    if (want) next.set('mode', want);
    else next.delete('mode');
    setParams(next, { replace: true });
  }, [value.mode, params, setParams]);

  const previewReq = useDebouncedValue<PreviewChangesRequest>(
    { base_version: base, ...manifestPart(value, draft) },
    300,
  );
  const summary = useChangesSummary(projectId, previewReq);
  const summaryError = summary.error
    ? (clientApiError(summary.error)?.error ??
      'The summary of changes isn’t available.')
    : null;
  const suggestion = suggestDescription(summary.data);
  const descriptionValue = description ?? suggestion;

  const manifestDirty =
    value.mode === 'toml'
      ? value.pixiToml.trim() !== draft.pixi_toml.trim()
      : !sameForm(value.form, draft.form) ||
        value.pixiToml.trim() !== draft.pixi_toml.trim();
  const dirty =
    !created &&
    (manifestDirty ||
      tags.length !== 1 ||
      tags[0] !== 'latest' ||
      description !== null);
  const guard = useUnsavedChangesGuard(dirty);

  const tagInvalid = tags.some((t) => tagError(t));
  const descriptionError =
    serverErrors.description ??
    (submitted && !descriptionValue.trim() ? 'Enter a description.' : null);
  const editorErrors: ManifestEditorErrors = {};
  for (const [field, message] of Object.entries(serverErrors)) {
    const key = EDITOR_FIELDS[field];
    if (key) editorErrors[key] = message;
  }
  if (
    submitted &&
    value.mode === 'form' &&
    value.form.platforms.length === 0 &&
    !editorErrors.platforms
  ) {
    editorErrors.platforms = 'Add at least one platform.';
  }

  const onEditorChange = (next: ManifestEditorValue) => {
    setValue(next);
    setServerErrors((e) => {
      const { description: d, tags: t } = e;
      const kept: Record<string, string> = {};
      if (d) kept.description = d;
      if (t) kept.tags = t;
      return kept;
    });
  };

  const startInstall = (n: number) => {
    detailApi
      .install(projectId, { version: n })
      .then((job) =>
        trackJobToast(job, {
          title: `Installing version ${n}`,
          successTitle: `Version ${n} installed`,
          successDescription: 'Ready to use on this machine.',
          errorTitle: `Version ${n} couldn’t install`,
          cancellable: true,
          onViewJob: (j) => navigate(`/jobs/${j.id}`),
        }),
      )
      .catch((err) =>
        toast.add({
          type: 'error',
          title: `Version ${n} couldn’t install`,
          description: clientApiError(err)?.error,
        }),
      );
  };

  // "Push to server" on the "Version N created" toast (linked projects).
  const pushUpTo = (n: number) => {
    remoteApi
      .push(projectId, n)
      .then((res) =>
        toast.add({
          type: 'success',
          title:
            res.pushed_versions.length > 0
              ? `${capitalize(versionList(res.pushed_versions))} pushed`
              : 'Already on the server',
          description: 'This machine and the server are in sync.',
        }),
      )
      .catch((err) =>
        toast.add({
          type: 'error',
          title: 'Couldn’t push to the server',
          description: clientApiError(err)?.error ?? 'Try again in a moment.',
        }),
      );
  };

  const submit = async (install: boolean) => {
    if (create.isPending) return;
    setSubmitted(true);
    setSubmitting(install);
    const desc = descriptionValue.trim();
    const noPlatforms =
      value.mode === 'form' && value.form.platforms.length === 0;
    if (!desc || tagInvalid || noPlatforms) {
      if (!desc) descriptionRef.current?.focus();
      return;
    }
    const changes = summarizeRequested(summary.data);
    try {
      const job = await create.mutateAsync({
        base_version: base,
        description: desc,
        tags: tagsForRequest(tags),
        ...manifestPart(value, draft),
      });
      setCreated(true);
      trackJobToast(job, {
        successTitle: (j) => `Version ${j.version_number ?? ''} created`,
        successDescription: [`Started from version ${base}`, ...changes].join(
          ' · ',
        ),
        cancellable: true,
        onViewJob: (j) => navigate(`/jobs/${j.id}`),
        successAction: (j) =>
          linked && !install && j.version_number != null
            ? {
                label: 'Push to server',
                onClick: () => pushUpTo(j.version_number as number),
              }
            : null,
        onSettled: (final) => {
          const n = final.version_number;
          if (final.status !== 'completed' || n == null) return;
          // Show the new version if the user is still on the project.
          if (window.location.pathname.endsWith(`/projects/${projectId}`)) {
            navigate(`/projects/${projectId}?v=${n}`, { replace: true });
          }
          if (install) startInstall(n);
        },
      });
      guard.navigateAway(`/projects/${projectId}`);
    } catch (err) {
      const e = clientApiError(err);
      if (e?.code === 'no_changes') {
        toast.add({
          type: 'info',
          title: e.error,
          description:
            'Change a package, channel or platform to create a new version.',
        });
      } else if (
        e?.code === 'invalid' &&
        e.field &&
        e.field !== 'base_version'
      ) {
        setServerErrors({ [e.field]: e.error });
        if (e.field === 'description') descriptionRef.current?.focus();
      } else {
        toast.add({
          type: 'error',
          title: 'Create version failed',
          description: e?.error ?? (err as Error).message,
        });
      }
    }
  };

  return (
    <main className="flex flex-1 flex-col px-9 pt-9">
      <form
        className="flex flex-1 flex-col"
        onSubmit={(e) => e.preventDefault()}
        noValidate
      >
        <div className={cn(PAGE_COLUMN, 'flex flex-col gap-1.5 pb-6')}>
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem>
                <BreadcrumbLink render={<Link to="/" />}>
                  Projects
                </BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbLink render={<Link to={detailPath} />}>
                  {name}
                </BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>New version</BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
          <div className="flex flex-col gap-1.5">
            <h1 className="font-bold text-3xl text-foreground">
              New version of {name}
            </h1>
            <p className="text-muted-foreground-strong text-sm">
              Starts from version {base}’s environment. Version {base} stays as
              it is.
            </p>
          </div>
        </div>

        <div className={PAGE_COLUMN}>
          <ManifestEditor
            value={value}
            onChange={onEditorChange}
            showName={false}
            disabled={create.isPending}
            errors={editorErrors}
          />
          <FormSection
            id="changes"
            title="Summary of changes"
            intro={`Compared with version ${base}.`}
          >
            <ChangesSummary
              projectName={name}
              diff={summary.data}
              loading={summary.isFetching}
              error={summaryError}
            />
          </FormSection>
          <FormSection
            id="about"
            title="About this version"
            intro="Shown in the version list and on the version’s page."
          >
            <Field
              invalid={!!descriptionError}
              className="w-full min-w-0 gap-1"
            >
              <FieldLabel className="text-foreground">Description</FieldLabel>
              <Input
                ref={descriptionRef}
                value={descriptionValue}
                onChange={(e) => {
                  setDescription(e.target.value);
                  setServerErrors(({ description: _d, ...rest }) => rest);
                }}
                placeholder="e.g., Swapped tensorflow for scikit-learn"
                autoComplete="off"
              />
              {descriptionError ? (
                <FieldError match className="text-xs">
                  {descriptionError}
                </FieldError>
              ) : description === null && suggestion ? (
                <FieldDescription>
                  Suggested from your changes.
                </FieldDescription>
              ) : null}
            </Field>
            <TagsInput
              label="Tags (optional)"
              value={tags}
              onChange={(next) => {
                setTags(next);
                setServerErrors(({ tags: _t, ...rest }) => rest);
              }}
              suggestions={draft.existing_tags}
              placeholder="Add a tag"
              description="A tag stays with this version until a newer version uses the same tag. latest does this automatically."
              invalid={tagError}
              error={serverErrors.tags}
            />
          </FormSection>
        </div>

        <footer className="sticky bottom-0 z-10 -mx-9 mt-auto bg-canvas px-9">
          <div
            className={cn(
              PAGE_COLUMN,
              'flex items-center justify-between gap-4 border-border border-t py-3',
            )}
          >
            <Button
              type="button"
              variant="ghost"
              onClick={() => guard.leave(detailPath)}
            >
              Cancel
            </Button>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="secondary"
                disabled={create.isPending && submitting !== false}
                loading={create.isPending && submitting === false}
                onClick={() => submit(false)}
              >
                Create version
              </Button>
              <Button
                type="button"
                disabled={create.isPending && submitting !== true}
                loading={create.isPending && submitting === true}
                onClick={() => submit(true)}
              >
                Create and install
              </Button>
            </div>
          </div>
        </footer>
      </form>
      <DiscardNewVersionDialog
        open={guard.pendingPath != null}
        onOpenChange={(open) => !open && guard.stay()}
        baseVersion={base}
        onDiscard={guard.discard}
      />
    </main>
  );
}
