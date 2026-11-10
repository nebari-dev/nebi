// Owner: U2 (detail). "Environment specification" panel: channels,
// platforms, Specified / All Resolved packages with search, a platform
// picker for resolved packages, and the Files menu (view pixi.toml /
// pixi.lock, download the version archive).
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
import { Input } from '@nebi/ui/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@nebi/ui/components/ui/select';
import { Spinner } from '@nebi/ui/components/ui/spinner';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@nebi/ui/components/ui/table';
import {
  Tabs,
  TabsIndicator,
  TabsList,
  TabsPanel,
  TabsTab,
} from '@nebi/ui/components/ui/tabs';
import { toast } from '@nebi/ui/components/ui/toast';
import { ChevronDown, CodeXml, Download, Search } from 'lucide-react';
import { useMemo, useState } from 'react';
import { clientApiError } from '@/api/client/base';
import {
  useDownloadArchive,
  useResolvedPackages,
  useVersion,
} from '@/hooks/client/detail';
import { useCapabilities } from '@/hooks/client/projects';
import type {
  ProjectListItem,
  ResolvedPackage,
  VersionItem,
} from '@/types/contracts';
import { FileDialog, type VersionFileName } from './FileDialog';
import { formatBytes, pixiPlatformFor, platformLabel } from './format';

export interface EnvironmentSpecProps {
  project: ProjectListItem;
  version: VersionItem;
}

const cellClass = 'h-10 py-2';

export const EnvironmentSpec = ({ project, version }: EnvironmentSpecProps) => {
  const n = version.number;
  const { data: detail, error: detailError } = useVersion(project.id, n);
  const { data: resolved, isLoading: resolvedLoading } = useResolvedPackages(
    project.id,
    n,
  );
  const { data: caps } = useCapabilities();
  const machine = pixiPlatformFor(caps?.platform);
  const download = useDownloadArchive();

  const [tab, setTab] = useState<'requested' | 'resolved'>('requested');
  const [query, setQuery] = useState('');
  const [file, setFile] = useState<VersionFileName | null>(null);
  const [chosenPlatform, setChosenPlatform] = useState<string | null>(null);

  const platforms = resolved?.platforms ?? [];
  const platform =
    chosenPlatform && platforms.includes(chosenPlatform)
      ? chosenPlatform
      : platforms.includes(machine)
        ? machine
        : (platforms[0] ?? '');
  const resolvedPkgs: ResolvedPackage[] = resolved?.packages[platform] ?? [];
  const resolvedByName = useMemo(
    () =>
      new Map(
        resolvedPkgs.map((p) => [
          `${p.source}:${p.name.toLowerCase()}`,
          p.version,
        ]),
      ),
    [resolvedPkgs],
  );

  const requested = detail?.spec.requested ?? [];
  const q = query.trim().toLowerCase();
  const shownRequested = requested.filter((r) =>
    r.name.toLowerCase().includes(q),
  );
  const shownResolved = resolvedPkgs.filter((r) =>
    r.name.toLowerCase().includes(q),
  );

  const downloadArchive = () =>
    download.mutate(
      { projectId: project.id, n },
      {
        onSuccess: (fileName) =>
          toast.add({
            type: 'success',
            title: 'Archive downloaded',
            description: fileName,
          }),
        onError: (err) =>
          toast.add({
            type: 'error',
            title: 'Couldn’t download the archive',
            description: clientApiError(err)?.error ?? 'Try again in a moment.',
          }),
      },
    );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h3 className="font-semibold text-base text-foreground leading-5">
          Environment specification
        </h3>
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger
            variant="outline"
            className="h-6 gap-1 px-2 text-xs [&_svg]:size-3"
          >
            Files
            <ChevronDown className="size-3" />
          </DropdownMenuTrigger>
          <DropdownMenuPortal>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setFile('pixi.toml')}>
                <CodeXml />
                View pixi.toml
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setFile('pixi.lock')}>
                <CodeXml />
                View pixi.lock
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={version.server_only || download.isPending}
                onClick={downloadArchive}
              >
                <Download />
                {version.server_only
                  ? `Download archive (pull version ${n} first)`
                  : 'Download archive'}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenuPortal>
        </DropdownMenu>
      </div>

      {detailError && !detail ? (
        <p className="text-destructive-foreground text-sm" role="alert">
          Couldn’t load version {n}.{' '}
          {clientApiError(detailError)?.error ?? 'Try again in a moment.'}
        </p>
      ) : (
        <dl className="grid grid-cols-[88px_1fr] gap-x-1.5 gap-y-2 text-sm">
          <dt className="text-muted-foreground">Channels</dt>
          <dd className="text-foreground">
            {detail ? detail.spec.channels.join(', ') || '–' : '…'}
          </dd>
          <dt className="text-muted-foreground">Platforms</dt>
          <dd className="text-foreground">
            {detail ? detail.spec.platforms.join(', ') || '–' : '…'}
          </dd>
        </dl>
      )}

      <div className="flex flex-col gap-2">
        <h4 className="font-semibold text-base text-foreground leading-5">
          Packages
        </h4>
        <Tabs
          value={tab}
          onValueChange={(v) => setTab(v as 'requested' | 'resolved')}
        >
          <div className="flex flex-wrap items-end justify-between gap-2 border-border border-b pb-1.5">
            <TabsList variant="underline" className="border-0">
              <TabsTab value="requested">
                Specified ({requested.length})
              </TabsTab>
              <TabsTab value="resolved">
                All Resolved ({resolvedPkgs.length})
              </TabsTab>
              <TabsIndicator />
            </TabsList>
            <div className="flex items-center gap-2">
              <div className="relative w-60">
                <Search
                  className="pointer-events-none absolute top-1/2 left-3 z-10 size-[18px] -translate-y-1/2 text-muted-foreground"
                  aria-hidden
                />
                <Input
                  type="search"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="Search packages"
                  aria-label="Search packages"
                  className="pl-9"
                />
              </div>
              {tab === 'resolved' && platforms.length > 0 && (
                <Select
                  value={platform}
                  onValueChange={(v) => v && setChosenPlatform(String(v))}
                >
                  <SelectTrigger className="w-auto" aria-label="Platform">
                    {/* Every label is laid out invisibly in the same cell so
                        the trigger keeps its width when the platform changes. */}
                    <span className="grid text-left">
                      {platforms.map((p) => (
                        <span
                          key={p}
                          aria-hidden
                          className="invisible col-start-1 row-start-1 whitespace-nowrap"
                        >
                          {platformLabel(p, machine)}
                        </span>
                      ))}
                      <SelectValue className="col-start-1 row-start-1 whitespace-nowrap">
                        {(v: string) => platformLabel(v, machine)}
                      </SelectValue>
                    </span>
                  </SelectTrigger>
                  <SelectContent className="w-auto min-w-(--anchor-width)">
                    {platforms.map((p) => (
                      <SelectItem key={p} value={p}>
                        {platformLabel(p, machine)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </div>
          </div>
          <TabsPanel value="requested" className="pt-1.5">
            <Table aria-label="Specified packages">
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Version constraint</TableHead>
                  <TableHead>Resolved version</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {shownRequested.map((r) => (
                  <TableRow key={`${r.source ?? 'conda'}:${r.name}`}>
                    <TableCell className={cellClass}>
                      {r.name}
                      {r.source === 'pypi' && (
                        <span className="ml-1.5 text-muted-foreground text-xs">
                          PyPI
                        </span>
                      )}
                    </TableCell>
                    <TableCell className={cellClass}>{r.constraint}</TableCell>
                    <TableCell className={cellClass}>
                      {resolvedByName.get(
                        `${r.source ?? 'conda'}:${r.name.toLowerCase()}`,
                      ) ?? '–'}
                    </TableCell>
                  </TableRow>
                ))}
                {detail && shownRequested.length === 0 && (
                  <EmptyRow
                    cols={3}
                    text={
                      q
                        ? `No packages match “${query}”.`
                        : 'This version doesn’t request any packages.'
                    }
                  />
                )}
              </TableBody>
            </Table>
          </TabsPanel>
          <TabsPanel value="resolved" className="pt-1.5">
            <Table aria-label="Resolved packages">
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Version</TableHead>
                  <TableHead>Channel</TableHead>
                  <TableHead>Download size</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {shownResolved.map((p) => (
                  <TableRow key={`${p.source}:${p.name}`}>
                    <TableCell className={cellClass}>{p.name}</TableCell>
                    <TableCell className={cellClass}>{p.version}</TableCell>
                    <TableCell className={cellClass}>{p.channel}</TableCell>
                    <TableCell className={cellClass}>
                      {formatBytes(p.size) || '–'}
                    </TableCell>
                  </TableRow>
                ))}
                {resolvedLoading && (
                  <TableRow>
                    <TableCell colSpan={4} className="h-20 text-center">
                      <Spinner className="inline" /> Loading packages…
                    </TableCell>
                  </TableRow>
                )}
                {!resolvedLoading && shownResolved.length === 0 && (
                  <EmptyRow
                    cols={4}
                    text={
                      q
                        ? `No packages match “${query}”.`
                        : platforms.length === 0
                          ? 'This version has no pixi.lock, so nothing is resolved yet.'
                          : `Nothing is resolved for ${platform}.`
                    }
                  />
                )}
              </TableBody>
            </Table>
            {platform && resolved?.total_size[platform] ? (
              <p className="mt-2 text-muted-foreground text-xs">
                {formatBytes(resolved.total_size[platform])} to download for{' '}
                {platform}
              </p>
            ) : null}
          </TabsPanel>
        </Tabs>
      </div>

      <FileDialog
        projectId={project.id}
        projectPath={project.path}
        version={n}
        file={file}
        onOpenChange={(open) => {
          if (!open) setFile(null);
        }}
      />
    </div>
  );
};

const EmptyRow = ({ cols, text }: { cols: number; text: string }) => (
  <TableRow>
    <TableCell
      colSpan={cols}
      className="h-20 text-center text-muted-foreground"
    >
      {text}
    </TableCell>
  </TableRow>
);
