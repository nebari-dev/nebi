import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { PublishDialog } from './PublishDialog';
import { Upload } from 'lucide-react';

interface PublishButtonProps {
  workspaceId: string;
  workspaceName: string;
  workspaceStatus: string;
}

export const PublishButton = ({ workspaceId, workspaceName, workspaceStatus }: PublishButtonProps) => {
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        onClick={() => setOpen(true)}
        disabled={workspaceStatus !== 'ready'}
        title={workspaceStatus !== 'ready' ? 'Workspace must be ready to publish' : 'Publish to OCI Registry'}
      >
        <Upload className="h-4 w-4 mr-2" />
        Publish
      </Button>
      <PublishDialog open={open} onOpenChange={setOpen} workspaceId={workspaceId} workspaceName={workspaceName} />
    </>
  );
};
