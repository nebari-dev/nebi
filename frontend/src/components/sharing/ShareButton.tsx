import { Users } from 'lucide-react';
import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { ShareDialog } from './ShareDialog';

interface ShareButtonProps {
  environmentId: string;
}

export const ShareButton = ({ environmentId }: ShareButtonProps) => {
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
        <Users className="h-4 w-4 mr-2" />
        Share
      </Button>
      <ShareDialog
        open={open}
        onOpenChange={setOpen}
        environmentId={environmentId}
      />
    </>
  );
};
