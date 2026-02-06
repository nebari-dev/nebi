import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { ShareDialog } from './ShareDialog';
import { Users } from 'lucide-react';

interface ShareButtonProps {
  workspaceId: string;
}

export const ShareButton = ({ workspaceId }: ShareButtonProps) => {
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
        <Users className="h-4 w-4 mr-2" />
        Share
      </Button>
      <ShareDialog open={open} onOpenChange={setOpen} workspaceId={workspaceId} />
    </>
  );
};
