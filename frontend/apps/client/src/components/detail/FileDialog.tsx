// Owner: U2 (detail). "pixi.toml · version N" / "pixi.lock · version N"
// viewer opened from the Files menu.
import { CodeBlock, CodeBlockBody } from '@nebi/ui/components/ui/code-block';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';
import { Spinner } from '@nebi/ui/components/ui/spinner';
import { clientApiError } from '@/api/client/base';
import { useVersionFile } from '@/hooks/client/detail';

export type VersionFileName = 'pixi.toml' | 'pixi.lock';

export interface FileDialogProps {
  projectId: string;
  projectPath: string;
  version: number;
  file: VersionFileName | null;
  onOpenChange: (open: boolean) => void;
}

export const FileDialog = ({
  projectId,
  projectPath,
  version,
  file,
  onOpenChange,
}: FileDialogProps) => {
  const name = file ?? 'pixi.toml';
  const { data, isLoading, error } = useVersionFile(
    projectId,
    version,
    name,
    file !== null,
  );
  const empty = data && data.content.trim() === '';
  return (
    <Dialog open={file !== null} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {name} · version {version}
          </DialogTitle>
          <DialogDescription>
            {projectPath ? `${projectPath.replace(/\/+$/, '')}/${name}` : name}
          </DialogDescription>
        </DialogHeader>
        {isLoading && (
          <div className="flex items-center gap-2 text-muted-foreground text-sm">
            <Spinner />
            Loading {name}…
          </div>
        )}
        {error && (
          <p className="text-destructive-foreground text-sm" role="alert">
            Couldn’t load {name}.{' '}
            {clientApiError(error)?.error ?? 'Try again in a moment.'}
          </p>
        )}
        {empty && (
          <p className="text-muted-foreground text-sm">
            Version {version} has no {name}.
          </p>
        )}
        {data && !empty && (
          <CodeBlock code={data.content} className="w-full">
            <CodeBlockBody maxLines={22} />
          </CodeBlock>
        )}
      </DialogContent>
    </Dialog>
  );
};
