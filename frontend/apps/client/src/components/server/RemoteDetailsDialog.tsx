// Owner: U5 (remote). "Remote details" of a server project: where it lives,
// who owns it, your role, and its versions with what is on this machine.
// Opened from the Server page row menu; U1 can open it from the Remotes
// column of the projects list: <RemoteDetailsDialog serverProjectId=… />.
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
import { Spinner } from '@nebi/ui/components/ui/spinner';
import { Laptop, Server } from 'lucide-react';
import { Link } from 'react-router-dom';
import { clientApiError } from '@/api/client/base';
import { useRemoteDetails } from '@/hooks/client/remote';
import { SHARE_ROLE_LABELS } from '@/types/contracts';
import { displayServerUrl } from './format';

export interface RemoteDetailsDialogProps {
  serverProjectId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const RemoteDetailsDialog = ({
  serverProjectId,
  open,
  onOpenChange,
}: RemoteDetailsDialogProps) => (
  <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="max-w-[560px]">
      {open && <Details serverProjectId={serverProjectId} />}
    </DialogContent>
  </Dialog>
);

const Details = ({ serverProjectId }: { serverProjectId: string }) => {
  const { data, isLoading, error } = useRemoteDetails(serverProjectId);
  if (isLoading) {
    return (
      <>
        <DialogHeader>
          <DialogTitle>Remote details</DialogTitle>
        </DialogHeader>
        <div className="flex justify-center py-6">
          <Spinner />
        </div>
      </>
    );
  }
  if (!data) {
    return (
      <>
        <DialogHeader>
          <DialogTitle>Remote details</DialogTitle>
          <DialogDescription>
            {clientApiError(error)?.error ??
              'The server project couldn’t be loaded.'}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <DialogClose render={<Button variant="secondary" />}>
            Close
          </DialogClose>
        </DialogFooter>
      </>
    );
  }
  const { project, versions } = data;
  const here = versions.filter((v) => !v.server_only).length;
  return (
    <>
      <DialogHeader>
        <DialogTitle>{project.name}</DialogTitle>
        <DialogDescription>
          On {displayServerUrl(data.url.replace(/\/projects\/.*$/, ''))}
        </DialogDescription>
      </DialogHeader>
      <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-sm">
        <dt className="font-medium text-foreground">Owner</dt>
        <dd className="text-foreground">{project.owner}</dd>
        <dt className="font-medium text-foreground">Namespace</dt>
        <dd className="text-foreground">{project.namespace}</dd>
        <dt className="font-medium text-foreground">Your role</dt>
        <dd className="text-foreground">{SHARE_ROLE_LABELS[project.role]}</dd>
        <dt className="font-medium text-foreground">On this machine</dt>
        <dd className="text-foreground">
          {project.local_project_id ? (
            <Link
              to={`/projects/${project.local_project_id}`}
              className="underline underline-offset-4"
            >
              {here} of {versions.length} versions
            </Link>
          ) : (
            'Not pulled yet'
          )}
        </dd>
      </dl>
      <section aria-labelledby="remote-versions-title" className="grid gap-2">
        <h3
          id="remote-versions-title"
          className="font-medium text-foreground text-sm"
        >
          Versions on the server
        </h3>
        {versions.length === 0 ? (
          <p className="text-muted-foreground text-sm">No versions yet.</p>
        ) : (
          <ul className="grid max-h-64 gap-1 overflow-y-auto">
            {versions.map((v) => (
              <li
                key={v.number}
                className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm"
              >
                <span className="font-medium">v{v.number}</span>
                {v.tags.map((t) => (
                  <Badge key={t} variant="outline">
                    {t}
                  </Badge>
                ))}
                <span className="min-w-0 flex-1 truncate text-muted-foreground-strong">
                  {v.description}
                </span>
                {v.server_only ? (
                  <span className="flex items-center gap-1 text-muted-foreground-strong text-xs">
                    <Server className="size-3.5" aria-hidden />
                    Server only
                  </span>
                ) : (
                  <span className="flex items-center gap-1 text-success-foreground text-xs">
                    <Laptop className="size-3.5" aria-hidden />
                    On this machine
                  </span>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>
      <DialogFooter>
        <DialogClose render={<Button variant="secondary" />}>Close</DialogClose>
      </DialogFooter>
    </>
  );
};
