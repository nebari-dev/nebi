// Owner: U2 (detail). The selected version: title with tags and badges,
// author / date / server / published line, description, the action
// toolbar (Compare versions, Create new version, Push, Publish, Install /
// Uninstall / Pull), install banners and the environment specification.
import { PersonAvatar } from '@nebi/ui/components/ui/avatar';
import { Badge } from '@nebi/ui/components/ui/badge';
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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@nebi/ui/components/ui/tooltip';
import { Check, GitCompare, Package, Plus, Server, Upload } from 'lucide-react';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { PublishDialog } from '@/components/publish/PublishDialog';
import type { ProjectInstallState } from '@/hooks/client/detail';
import type { ProjectListItem, VersionItem } from '@/types/contracts';
import { EnvironmentSpec } from './EnvironmentSpec';
import {
  authorLabel,
  formatVersionDate,
  publishedReferences,
  serverRemote,
  versionsPhrase,
} from './format';
import {
  InstallBanner,
  InstallControls,
  InstallFailedAlert,
} from './InstallStatus';
import { PushButton } from './RemoteActions';

export interface VersionPanelProps {
  project: ProjectListItem;
  version: VersionItem;
  versions: VersionItem[];
  installState: ProjectInstallState;
  onInstall: (version: number) => void;
  installPending: boolean;
}

const Dot = () => (
  <span className="text-muted-foreground" aria-hidden>
    ·
  </span>
);

export const VersionPanel = ({
  project,
  version,
  versions,
  installState,
  onInstall,
  installPending,
}: VersionPanelProps) => {
  const navigate = useNavigate();
  const [olderBaseOpen, setOlderBaseOpen] = useState(false);
  const [publishOpen, setPublishOpen] = useState(false);
  const n = version.number;
  const serverOnly = version.server_only;
  const localNumbers = versions
    .filter((v) => !v.server_only)
    .map((v) => v.number);
  const latestLocal = localNumbers.length ? Math.max(...localNumbers) : n;
  const newer = localNumbers.filter((x) => x > n).sort((a, b) => a - b);
  const newVersionPath = (from: number) =>
    `/projects/${project.id}/new-version?from=${from}`;
  const createNewVersion = () =>
    newer.length > 0 ? setOlderBaseOpen(true) : navigate(newVersionPath(n));
  const installed = project.installed_version === n;

  return (
    <section
      className="flex min-w-0 flex-1 flex-col gap-4"
      aria-label={`Version ${n}`}
    >
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-2">
          <div className="flex min-h-7 flex-wrap items-center gap-2">
            <h2 className="font-semibold text-foreground text-xl leading-7">
              Version {n}
            </h2>
            {version.tags.map((t) => (
              <Badge key={t} variant="outline">
                {t}
              </Badge>
            ))}
            {version.published.length > 0 && (
              <Badge variant="secondary">
                <Package />
                Published
              </Badge>
            )}
            {installed && (
              <Badge variant="success" data-tour="installed">
                <Check />
                Installed
              </Badge>
            )}
          </div>
          <VersionMeta project={project} version={version} />
        </div>
        {version.description && (
          <p className="text-base text-foreground">{version.description}</p>
        )}
        <div
          className="flex flex-wrap items-center gap-2"
          role="toolbar"
          aria-label={`Version ${n} actions`}
        >
          <Button
            variant="secondary"
            size="sm"
            onClick={() =>
              navigate(`/projects/${project.id}/compare?from=${n}`)
            }
          >
            <GitCompare />
            Compare versions
          </Button>
          <Tooltip>
            {/* The wrapper keeps the tooltip working on the disabled button. */}
            <TooltipTrigger render={<span className="inline-flex" />}>
              <Button
                variant="secondary"
                size="sm"
                disabled={serverOnly}
                onClick={createNewVersion}
                data-tour="create-version"
              >
                <Plus />
                Create new version
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {serverOnly
                ? `Pull version ${n} first`
                : `Creates a new version based on version ${n}`}
            </TooltipContent>
          </Tooltip>
          <PushButton project={project} version={version} versions={versions} />
          <Button
            variant="secondary"
            size="sm"
            disabled={serverOnly}
            onClick={() => setPublishOpen(true)}
            data-tour="publish"
          >
            <Upload />
            Publish
          </Button>
          <InstallControls
            project={project}
            version={version}
            state={installState}
            onInstall={onInstall}
            installPending={installPending}
          />
        </div>
        <InstallBanner
          project={project}
          version={version}
          state={installState}
        />
        <InstallFailedAlert
          version={version}
          state={installState}
          onRetry={onInstall}
        />
      </div>
      <hr className="border-border" />
      <EnvironmentSpec project={project} version={version} />

      {!serverOnly && (
        <PublishDialog
          projectId={project.id}
          projectName={project.name}
          version={version}
          open={publishOpen}
          onOpenChange={setPublishOpen}
        />
      )}
      <Dialog open={olderBaseOpen} onOpenChange={setOlderBaseOpen}>
        <DialogContent className="max-w-[520px]">
          <DialogHeader>
            <DialogTitle>Start a new version from version {n}?</DialogTitle>
            <DialogDescription>
              Version {n} is older than the latest version, v{latestLocal}.
            </DialogDescription>
          </DialogHeader>
          <p className="text-foreground text-sm">
            The new version starts with version {n}’s packages and settings.
            Changes made in {versionsPhrase(newer.length ? newer : [n])} won’t
            carry over, but those versions stay in the history.
          </p>
          <DialogFooter>
            <DialogClose
              render={<Button variant="ghost" className="sm:mr-auto" />}
            >
              Cancel
            </DialogClose>
            <Button
              variant="secondary"
              onClick={() => navigate(newVersionPath(latestLocal))}
            >
              Start from v{latestLocal}
            </Button>
            <Button onClick={() => navigate(newVersionPath(n))}>
              Start from version {n}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
};

/** Author · date, "On the server", "Published to ...". */
const VersionMeta = ({
  project,
  version,
}: {
  project: ProjectListItem;
  version: VersionItem;
}) => {
  const linked = serverRemote(project) !== null;
  const onServer =
    version.server_only ||
    (linked &&
      project.server_latest_version != null &&
      version.number <= project.server_latest_version);
  const refs = publishedReferences(version);
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-muted-foreground-strong text-sm">
      <span className="flex items-center gap-1.5">
        <PersonAvatar
          name={version.author.name}
          className="size-5 text-[10px]"
        />
        {authorLabel(version)} · {formatVersionDate(version.created_at)}
      </span>
      {onServer && (
        <>
          <Dot />
          <span className="flex items-center gap-1.5">
            <Server className="size-3.5" aria-hidden />
            {version.server_only
              ? 'On the server · not on this machine yet'
              : 'On the server'}
          </span>
        </>
      )}
      {refs.length > 0 && (
        <>
          <Dot />
          {refs.length === 1 ? (
            <span className="flex items-center gap-1.5">
              <Package className="size-3.5" aria-hidden />
              Published to {refs[0]}
            </span>
          ) : (
            <Tooltip>
              <TooltipTrigger
                render={
                  <button
                    type="button"
                    className="flex items-center gap-1.5 underline decoration-dotted underline-offset-4 outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  />
                }
              >
                <Package className="size-3.5" aria-hidden />
                Published to {refs.length} registries
              </TooltipTrigger>
              <TooltipContent>{refs.join(', ')}</TooltipContent>
            </Tooltip>
          )}
        </>
      )}
    </div>
  );
};
