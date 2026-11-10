// Owner: U5 (remote). "Pull <project>" / "Pull and install <project>" with
// the local name (prototype PullAndInstall). The name check runs on the
// backend (409 name_taken) and, when the projects list is loaded, here too.
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
import { type FormEvent, useState } from 'react';
import { clientApiError } from '@/api/client/base';
import { usePullProject } from '@/hooks/client/remote';
import type {
  ProjectListItem,
  PullResult,
  ServerProjectItem,
} from '@/types/contracts';
import { nameTakenMessage } from './copy';
import { TextField } from './TextField';

export interface PullDialogProps {
  project: ServerProjectItem;
  andInstall: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Local projects, for an instant name check (optional). */
  localProjects?: ProjectListItem[];
  /** The pull job was submitted. */
  onPullStarted?: (result: PullResult, localName: string) => void;
}

export const PullDialog = ({
  project,
  andInstall,
  open,
  onOpenChange,
  localProjects,
  onPullStarted,
}: PullDialogProps) => (
  <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="max-w-[520px]">
      {open && (
        <PullForm
          project={project}
          andInstall={andInstall}
          localProjects={localProjects}
          onStarted={(res, name) => {
            onOpenChange(false);
            onPullStarted?.(res, name);
          }}
        />
      )}
    </DialogContent>
  </Dialog>
);

const PullForm = ({
  project,
  andInstall,
  localProjects,
  onStarted,
}: {
  project: ServerProjectItem;
  andInstall: boolean;
  localProjects?: ProjectListItem[];
  onStarted: (res: PullResult, name: string) => void;
}) => {
  const [name, setName] = useState(project.name);
  const [error, setError] = useState<string>();
  const pull = usePullProject();
  const action = andInstall ? 'Pull and install' : 'Pull';

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const local = name.trim();
    if (!local) {
      setError('Give the project a name.');
      return;
    }
    if (
      localProjects?.some((p) => p.name.toLowerCase() === local.toLowerCase())
    ) {
      setError(nameTakenMessage(local));
      return;
    }
    pull.mutate(
      { serverProjectId: project.id, local_name: local, install: andInstall },
      {
        onSuccess: (res) => onStarted(res, local),
        onError: (err) =>
          setError(clientApiError(err)?.error ?? 'Couldn’t start the pull.'),
      },
    );
  };

  return (
    <form onSubmit={submit} className="grid gap-4" noValidate>
      <DialogHeader>
        <DialogTitle>
          {action} {project.name}
        </DialogTitle>
        <DialogDescription>
          Choose a name for this project on your machine.
          {andInstall && ' The environment will install after pulling.'}
        </DialogDescription>
      </DialogHeader>
      <TextField
        label="Local name"
        error={error}
        value={name}
        onChange={(e) => {
          setName(e.target.value);
          setError(undefined);
        }}
        onFocus={(e) => e.currentTarget.select()}
        autoFocus
        spellCheck={false}
      />
      <DialogFooter>
        <DialogClose render={<Button variant="secondary" />}>
          Cancel
        </DialogClose>
        <Button
          render={<button type="submit" />}
          loading={pull.isPending}
          loadingText="Pulling…"
        >
          {action}
        </Button>
      </DialogFooter>
    </form>
  );
};
