// Owner: U1 (projects). "No projects yet" (prototype EmptyNotConnected /
// EmptyConnected): New project plus the ways to bring a project in, which
// open the dialogs and pages other units own.
import { Button } from '@nebi/ui/components/ui/button';
import { Plus } from 'lucide-react';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { AddRegistryDialog } from '@/components/registries/AddRegistryDialog';
import { PullFromRegistryDialog } from '@/components/registries/PullFromRegistryDialog';
import { ConnectServerDialog } from '@/components/server/ConnectServerDialog';

export const ProjectsEmptyState = ({
  serverConnected,
}: {
  serverConnected: boolean;
}) => {
  const navigate = useNavigate();
  const [dialog, setDialog] = useState<
    'connect' | 'add-registry' | 'pull-registry' | null
  >(null);
  const close = (open: boolean) => !open && setDialog(null);

  return (
    <section
      className="mt-20 flex flex-col items-center gap-3 text-center"
      aria-labelledby="empty-title"
    >
      <h2
        id="empty-title"
        className="font-semibold text-foreground text-xl leading-7"
      >
        No projects yet
      </h2>
      <p className="max-w-xl text-muted-foreground-strong text-sm">
        {serverConnected
          ? 'Create one locally, or pull an existing project from your server or a registry.'
          : 'Create a project locally, or bring one in from your team’s server or a registry.'}
      </p>
      <div className="mt-2 flex flex-wrap justify-center gap-2">
        <Button onClick={() => navigate('/projects/new')}>
          <Plus />
          New project
        </Button>
        {serverConnected ? (
          <>
            <Button variant="secondary" onClick={() => navigate('/server')}>
              Pull from server
            </Button>
            <Button
              variant="secondary"
              onClick={() => setDialog('pull-registry')}
            >
              Pull from registry
            </Button>
          </>
        ) : (
          <>
            <Button variant="secondary" onClick={() => setDialog('connect')}>
              Connect to server
            </Button>
            <Button
              variant="secondary"
              onClick={() => setDialog('add-registry')}
            >
              Add a registry
            </Button>
          </>
        )}
      </div>
      <ConnectServerDialog open={dialog === 'connect'} onOpenChange={close} />
      <AddRegistryDialog
        open={dialog === 'add-registry'}
        onOpenChange={close}
      />
      <PullFromRegistryDialog
        open={dialog === 'pull-registry'}
        onOpenChange={close}
        onPulled={(projectId) => navigate(`/projects/${projectId}`)}
      />
    </section>
  );
};
