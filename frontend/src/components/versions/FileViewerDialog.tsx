import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Loader2, Copy, Download } from 'lucide-react';
import { useState } from 'react';

interface FileViewerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  content?: string;
  isLoading: boolean;
  fileName: string;
  onDownload: () => void;
}

export const FileViewerDialog = ({
  open,
  onOpenChange,
  title,
  content,
  isLoading,
  fileName,
  onDownload,
}: FileViewerDialogProps) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    if (content) {
      await navigator.clipboard.writeText(content);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl max-h-[80vh] flex flex-col">
        <DialogHeader>
          <div className="flex items-center justify-between">
            <DialogTitle>{title}</DialogTitle>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={handleCopy}
                disabled={isLoading || !content}
              >
                <Copy className="h-4 w-4 mr-2" />
                {copied ? 'Copied!' : 'Copy'}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={onDownload}
                disabled={isLoading}
              >
                <Download className="h-4 w-4 mr-2" />
                Download
              </Button>
            </div>
          </div>
        </DialogHeader>

        <div className="flex-1 overflow-auto border rounded-md bg-muted/30">
          {isLoading ? (
            <div className="flex items-center justify-center h-64">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          ) : content ? (
            <pre className="p-4 text-xs font-mono overflow-x-auto">
              <code>{content}</code>
            </pre>
          ) : (
            <div className="flex items-center justify-center h-64 text-muted-foreground">
              No content available
            </div>
          )}
        </div>

        <div className="text-xs text-muted-foreground">
          {fileName}
        </div>
      </DialogContent>
    </Dialog>
  );
};
