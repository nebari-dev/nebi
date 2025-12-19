import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { PublishDialog } from './PublishDialog';
import { Upload } from 'lucide-react';

interface PublishButtonProps {
  environmentId: string;
  environmentStatus: string;
}

export const PublishButton = ({ environmentId, environmentStatus }: PublishButtonProps) => {
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        onClick={() => setOpen(true)}
        disabled={environmentStatus !== 'ready'}
        title={environmentStatus !== 'ready' ? 'Environment must be ready to publish' : 'Publish to OCI Registry'}
      >
        <Upload className="h-4 w-4 mr-2" />
        Publish
      </Button>
      <PublishDialog open={open} onOpenChange={setOpen} environmentId={environmentId} />
    </>
  );
};
