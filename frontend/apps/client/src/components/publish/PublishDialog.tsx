// Owner: U7 (publish). Opened from the version rail (U2). Props are a
// cross-unit contract (CONTRACTS.md).
//
// Prototype screens PublishModal (one row per registry with a switch,
// editable repository:tag, "Will be unpublished" warning, Save enabled
// only when something changes) and PublishAfterSave (per-registry results).
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
import { Spinner } from '@nebi/ui/components/ui/spinner';
import { Switch } from '@nebi/ui/components/ui/switch';
import { toast } from '@nebi/ui/components/ui/toast';
import { CircleCheck, CircleX } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { clientApiError } from '@/api/client/base';
import { AddRegistryDialog } from '@/components/registries/AddRegistryDialog';
import {
  usePublish,
  usePublishDefaults,
  useRegistries,
  useUnpublish,
} from '@/hooks/client/publish';
import {
  formatReference,
  parseReference,
  REFERENCE_HINT,
  type RepositoryTag,
  registryHost,
} from '@/types/client/publish';
import type {
  PublishDefault,
  PublishedRef,
  RegistryItem,
  VersionItem,
} from '@/types/contracts';

export interface PublishDialogProps {
  projectId: string;
  projectName: string;
  version: VersionItem;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** One switch row: an existing publication or a registry to publish to. */
interface Row {
  key: string;
  registry: RegistryItem | null;
  name: string;
  host: string;
  ref: RepositoryTag;
  published: PublishedRef | null;
  /** Why the switch is locked, if it is. */
  locked: string | null;
}

interface RowState {
  on: boolean;
  ref: RepositoryTag;
}

interface SaveResult {
  key: string;
  name: string;
  host: string;
  reference: string;
  action: 'publish' | 'unpublish';
  ok: boolean;
  error?: string;
}

const slug = (name: string) =>
  name
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, '-')
    .replace(/^[-._]+|[-._]+$/g, '') || 'project';

function buildRows(
  registries: RegistryItem[],
  published: PublishedRef[],
  defaults: PublishDefault[],
  projectName: string,
  version: number,
): Row[] {
  const rows: Row[] = [];
  const claimed = new Set<string>();
  for (const reg of registries) {
    const host = registryHost(reg.url);
    const pubs = published.filter(
      (p) => p.source === 'local' && p.registry_id === reg.id,
    );
    for (const p of pubs) {
      if (p.publication_id) claimed.add(p.publication_id);
      rows.push({
        key: p.publication_id ?? `${reg.id}:${p.reference}`,
        registry: reg,
        name: reg.name,
        host: p.registry || host,
        ref: { repository: p.repository, tag: p.tag },
        published: p,
        locked: p.can_unpublish
          ? null
          : (reg.unpublish_reason ??
            'This registry can’t delete tags, so Nebi can’t unpublish from it.'),
      });
    }
    if (pubs.length === 0) {
      const d = defaults.find((x) => x.registry_id === reg.id);
      rows.push({
        key: `new:${reg.id}`,
        registry: reg,
        name: reg.name,
        host,
        ref: d
          ? { repository: d.repository, tag: d.tag }
          : {
              repository: reg.namespace
                ? `${reg.namespace}/${slug(projectName)}`
                : slug(projectName),
              tag: String(version),
            },
        published: null,
        locked: null,
      });
    }
  }
  // Publications this machine cannot manage: made on another machine
  // (server records) or on a registry that is no longer configured.
  for (const p of published) {
    if (p.publication_id && claimed.has(p.publication_id)) continue;
    rows.push({
      key: p.publication_id ?? `${p.source}:${p.reference}`,
      registry: null,
      name: p.registry,
      host: p.registry,
      ref: { repository: p.repository, tag: p.tag },
      published: p,
      locked:
        p.source === 'server'
          ? 'Published from another machine. Unpublish it there.'
          : 'This registry is no longer set up on this machine.',
    });
  }
  return rows;
}

function errorMessage(err: unknown, fallback: string): string {
  return clientApiError(err)?.error ?? fallback;
}

export const PublishDialog = ({
  projectId,
  projectName,
  version,
  open,
  onOpenChange,
}: PublishDialogProps) => {
  const registries = useRegistries();
  const defaults = usePublishDefaults(
    projectId,
    open ? version.number : undefined,
  );
  const publish = usePublish(projectId);
  const unpublish = useUnpublish(projectId);
  const [overrides, setOverrides] = useState<Record<string, RowState>>({});
  const [editing, setEditing] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [results, setResults] = useState<SaveResult[] | null>(null);
  const [addOpen, setAddOpen] = useState(false);

  useEffect(() => {
    if (open) {
      setOverrides({});
      setEditing(null);
      setResults(null);
    }
  }, [open]);

  const rows = useMemo(
    () =>
      buildRows(
        registries.data ?? [],
        version.published ?? [],
        defaults.data ?? [],
        projectName,
        version.number,
      ),
    [
      registries.data,
      version.published,
      defaults.data,
      projectName,
      version.number,
    ],
  );
  const stateOf = (row: Row): RowState =>
    overrides[row.key] ?? { on: !!row.published, ref: row.ref };
  const update = (row: Row, patch: Partial<RowState>) =>
    setOverrides((o) => ({ ...o, [row.key]: { ...stateOf(row), ...patch } }));

  const changed = rows.some((r) => stateOf(r).on !== !!r.published);
  const loading =
    registries.isPending ||
    (defaults.isPending && defaults.fetchStatus !== 'idle');

  const save = async () => {
    const toPublish = rows.filter(
      (r) => r.registry && !r.published && stateOf(r).on,
    );
    const toUnpublish = rows.filter(
      (r) => r.published?.publication_id && !stateOf(r).on,
    );
    setSaving(true);
    const out: SaveResult[] = [];
    if (toPublish.length > 0) {
      try {
        const res = await publish.mutateAsync({
          n: version.number,
          targets: toPublish.map((r) => ({
            registry_id: r.registry?.id as string,
            ...stateOf(r).ref,
          })),
        });
        res.results.forEach((item, i) => {
          const row = toPublish[i];
          out.push({
            key: row.key,
            name: item.registry_name || row.name,
            host: row.host,
            reference:
              item.reference || formatReference(row.host, stateOf(row).ref),
            action: 'publish',
            ok: item.status === 'published',
            error: item.error,
          });
        });
      } catch (err) {
        const message = errorMessage(err, 'Couldn’t publish this version.');
        for (const row of toPublish) {
          out.push({
            key: row.key,
            name: row.name,
            host: row.host,
            reference: formatReference(row.host, stateOf(row).ref),
            action: 'publish',
            ok: false,
            error: message,
          });
        }
      }
    }
    for (const row of toUnpublish) {
      const reference =
        row.published?.reference ?? formatReference(row.host, row.ref);
      try {
        await unpublish.mutateAsync(row.published?.publication_id as string);
        out.push({
          key: row.key,
          name: row.name,
          host: row.host,
          reference,
          action: 'unpublish',
          ok: true,
        });
      } catch (err) {
        out.push({
          key: row.key,
          name: row.name,
          host: row.host,
          reference,
          action: 'unpublish',
          ok: false,
          error: errorMessage(err, 'Couldn’t unpublish this tag.'),
        });
      }
    }
    setSaving(false);
    setResults(out);
    announce(version.number, out);
  };

  const failed = results?.some((r) => !r.ok) ?? false;

  return (
    <>
      <Dialog
        open={open && !addOpen}
        onOpenChange={(o) => !saving && onOpenChange(o)}
      >
        <DialogContent className="max-w-[520px]">
          <DialogHeader>
            <DialogTitle>Publish version {version.number}</DialogTitle>
            {results && (
              <DialogDescription>
                {failed
                  ? 'Some registries didn’t take the change. The others are done.'
                  : 'Every registry is up to date.'}
              </DialogDescription>
            )}
          </DialogHeader>

          {results ? (
            <ResultsList results={results} />
          ) : loading ? (
            <p className="flex items-center gap-2 py-3 text-muted-foreground text-sm">
              <Spinner className="size-4" /> Loading registries…
            </p>
          ) : registries.isError ? (
            <p
              role="alert"
              className="py-3 text-destructive-foreground text-sm"
            >
              {errorMessage(registries.error, 'Couldn’t load your registries.')}
            </p>
          ) : rows.length === 0 ? (
            <section className="flex flex-col items-start gap-2 py-3">
              <h3 className="font-medium text-foreground text-sm">
                No registries yet
              </h3>
              <p className="text-muted-foreground text-sm">
                Add a registry to publish versions to it.
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
            <ul className="flex flex-col divide-y divide-border">
              {rows.map((row) => {
                const st = stateOf(row);
                const willUnpublish = !!row.published && !st.on;
                const labelId = `publish-${row.key}`;
                return (
                  <li
                    key={row.key}
                    className="flex items-center justify-between gap-4 py-3"
                  >
                    <div className="flex min-w-0 flex-col gap-1">
                      <span
                        id={labelId}
                        className="font-medium text-foreground text-sm"
                      >
                        {row.name}
                      </span>
                      {editing === row.key ? (
                        <ReferenceEditor
                          host={row.host}
                          value={st.ref}
                          onDone={(ref) => {
                            if (ref) update(row, { ref });
                            setEditing(null);
                          }}
                        />
                      ) : (
                        <span className="flex items-center gap-3">
                          <code className="truncate font-mono text-muted-foreground text-xs">
                            {formatReference(row.host, st.ref)}
                          </code>
                          {st.on && !row.published && (
                            <Button
                              variant="link"
                              size="xs"
                              className="h-auto px-0"
                              disabled={saving}
                              onClick={() => setEditing(row.key)}
                            >
                              Edit
                            </Button>
                          )}
                        </span>
                      )}
                      {willUnpublish && (
                        <p className="text-destructive-foreground text-xs">
                          Will be unpublished. Pulls of this tag will stop
                          working.
                        </p>
                      )}
                      {row.locked && (
                        <p className="text-muted-foreground text-xs">
                          {row.locked}
                        </p>
                      )}
                    </div>
                    <Switch
                      aria-labelledby={labelId}
                      checked={st.on}
                      disabled={saving || !!row.locked}
                      onCheckedChange={(on) => update(row, { on })}
                    />
                  </li>
                );
              })}
            </ul>
          )}

          <DialogFooter>
            {results ? (
              <>
                {failed && (
                  <Button
                    variant="secondary"
                    onClick={() => {
                      setOverrides({});
                      setResults(null);
                    }}
                  >
                    Back
                  </Button>
                )}
                <Button onClick={() => onOpenChange(false)}>Done</Button>
              </>
            ) : (
              <>
                <DialogClose
                  render={<Button variant="secondary" />}
                  disabled={saving}
                >
                  Cancel
                </DialogClose>
                <Button
                  disabled={!changed || editing !== null}
                  loading={saving}
                  loadingText="Saving…"
                  onClick={save}
                >
                  Save
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <AddRegistryDialog open={open && addOpen} onOpenChange={setAddOpen} />
    </>
  );
};

/** Toasts for a save, worded like the prototype. */
function announce(n: number, results: SaveResult[]) {
  const hosts = (rs: SaveResult[]) =>
    [...new Set(rs.map((r) => r.host))].join(', ');
  const published = results.filter((r) => r.ok && r.action === 'publish');
  const unpublished = results.filter((r) => r.ok && r.action === 'unpublish');
  if (published.length > 0 || unpublished.length > 0) {
    toast.add({
      type: 'success',
      title:
        published.length > 0
          ? `Version ${n} published to ${hosts(published)}`
          : `Version ${n} unpublished from ${hosts(unpublished)}`,
      description:
        published.length > 0 && unpublished.length > 0
          ? `Unpublished from ${hosts(unpublished)}.`
          : undefined,
    });
  }
  const failedPublish = results.filter((r) => !r.ok && r.action === 'publish');
  const failedUnpublish = results.filter(
    (r) => !r.ok && r.action === 'unpublish',
  );
  if (failedPublish.length > 0) {
    toast.add({
      type: 'error',
      title: `Couldn’t publish version ${n} to ${hosts(failedPublish)}`,
      description: failedPublish[0].error,
    });
  }
  if (failedUnpublish.length > 0) {
    toast.add({
      type: 'error',
      title: `Couldn’t unpublish version ${n} from ${hosts(failedUnpublish)}`,
      description: failedUnpublish[0].error,
    });
  }
}

/** PublishAfterSave: one line per registry with its outcome. */
const ResultsList = ({ results }: { results: SaveResult[] }) => (
  <ul
    className="flex flex-col divide-y divide-border"
    aria-label="Publish results"
  >
    {results.map((r) => (
      <li key={`${r.action}:${r.key}`} className="flex items-start gap-3 py-3">
        {r.ok ? (
          <CircleCheck
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-success-foreground"
          />
        ) : (
          <CircleX
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-destructive-foreground"
          />
        )}
        <div className="flex min-w-0 flex-col gap-1">
          <span className="font-medium text-foreground text-sm">{r.name}</span>
          <code className="truncate font-mono text-muted-foreground text-xs">
            {r.reference}
          </code>
          <p
            className={
              r.ok
                ? 'text-muted-foreground-strong text-xs'
                : 'text-destructive-foreground text-xs'
            }
          >
            {r.ok
              ? r.action === 'publish'
                ? 'Published'
                : 'Unpublished. Pulls of this tag no longer work.'
              : `${r.action === 'publish' ? 'Couldn’t publish' : 'Couldn’t unpublish'}: ${r.error ?? 'unknown error'}`}
          </p>
        </div>
      </li>
    ))}
  </ul>
);

/** Inline repository:tag editor (prototype "Edit"). */
const ReferenceEditor = ({
  host,
  value,
  onDone,
}: {
  host: string;
  value: RepositoryTag;
  onDone: (ref: RepositoryTag | null) => void;
}) => {
  const [text, setText] = useState(`${value.repository}:${value.tag}`);
  const parsed = parseReference(text);
  const commit = () => onDone(parsed);
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-1">
        <span className="font-mono text-muted-foreground text-xs">{host}/</span>
        <Input
          autoFocus
          value={text}
          onChange={(e) => setText(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              commit();
            }
            if (e.key === 'Escape') {
              e.stopPropagation();
              onDone(null);
            }
          }}
          aria-label="Repository and tag"
          aria-invalid={!parsed}
          className="h-7 w-64 font-mono text-xs"
          spellCheck={false}
        />
      </div>
      {!parsed && (
        <p className="text-destructive-foreground text-xs">{REFERENCE_HINT}</p>
      )}
    </div>
  );
};
