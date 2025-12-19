import { useState } from 'react';
import { usePublicRegistries, usePublishEnvironment } from '@/hooks/useRegistries';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Loader2, Upload, AlertCircle } from 'lucide-react';

interface PublishDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  environmentId: string;
}

export const PublishDialog = ({ open, onOpenChange, environmentId }: PublishDialogProps) => {
  const { data: registries, isLoading: registriesLoading } = usePublicRegistries();
  const publishMutation = usePublishEnvironment();

  const [selectedRegistry, setSelectedRegistry] = useState('');
  const [repository, setRepository] = useState('');
  const [tag, setTag] = useState('');
  const [error, setError] = useState('');
  const [publishSuccess, setPublishSuccess] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    if (!selectedRegistry || !repository.trim() || !tag.trim()) {
      setError('All fields are required');
      return;
    }

    try {
      await publishMutation.mutateAsync({
        environmentId,
        data: {
          registry_id: selectedRegistry,
          repository: repository.trim(),
          tag: tag.trim(),
        },
      });
      setPublishSuccess(true);
      setTimeout(() => {
        onOpenChange(false);
        setPublishSuccess(false);
        setSelectedRegistry('');
        setRepository('');
        setTag('');
        // Refresh the page to show the new publication in the Publications tab
        window.location.reload();
      }, 2000);
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to publish environment. Please try again.';
      setError(errorMessage);
      console.error('Failed to publish:', err);
    }
  };

  const handleClose = () => {
    if (!publishMutation.isPending) {
      onOpenChange(false);
      setError('');
      setPublishSuccess(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Publish Environment to OCI Registry</DialogTitle>
          <DialogDescription>
            Publish the environment's pixi.toml and pixi.lock files as an OCI artifact.
          </DialogDescription>
        </DialogHeader>

        {publishSuccess ? (
          <div className="py-8 text-center">
            <div className="flex justify-center mb-4">
              <div className="h-12 w-12 rounded-full bg-green-500/10 flex items-center justify-center">
                <Upload className="h-6 w-6 text-green-500" />
              </div>
            </div>
            <p className="text-lg font-medium mb-2">Published successfully!</p>
            <p className="text-sm text-muted-foreground">
              Check the Publications tab to see your published artifact.
            </p>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4 mt-4">
            {registriesLoading ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            ) : registries && registries.length === 0 ? (
              <div className="bg-yellow-500/10 border border-yellow-500/20 text-yellow-500 px-4 py-3 rounded flex items-start gap-3">
                <AlertCircle className="h-5 w-5 flex-shrink-0 mt-0.5" />
                <div>
                  <p className="font-medium">No registries configured</p>
                  <p className="text-sm mt-1">
                    Contact your administrator to set up OCI registries for publishing.
                  </p>
                </div>
              </div>
            ) : (
              <>
                <div className="space-y-2">
                  <label className="text-sm font-medium">Registry</label>
                  <select
                    value={selectedRegistry}
                    onChange={(e) => setSelectedRegistry(e.target.value)}
                    className="w-full h-10 px-3 rounded-md border border-input bg-background"
                    required
                    autoFocus
                  >
                    <option value="">Select a registry</option>
                    {registries?.map((registry) => (
                      <option key={registry.id} value={registry.id}>
                        {registry.name} ({registry.url})
                      </option>
                    ))}
                  </select>
                </div>

                <div className="space-y-2">
                  <label className="text-sm font-medium">Repository</label>
                  <Input
                    type="text"
                    value={repository}
                    onChange={(e) => setRepository(e.target.value)}
                    placeholder="e.g., myorg/myenv or username/project"
                    required
                  />
                  <p className="text-xs text-muted-foreground">
                    The repository path in the registry
                  </p>
                </div>

                <div className="space-y-2">
                  <label className="text-sm font-medium">Tag</label>
                  <Input
                    type="text"
                    value={tag}
                    onChange={(e) => setTag(e.target.value)}
                    placeholder="e.g., v1.0.0 or latest"
                    required
                  />
                  <p className="text-xs text-muted-foreground">
                    Version tag for this publication
                  </p>
                </div>

                {error && (
                  <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-3 py-2 rounded text-sm">
                    {error}
                  </div>
                )}

                <div className="flex gap-2 justify-end pt-4">
                  <Button type="button" variant="outline" onClick={handleClose} disabled={publishMutation.isPending}>
                    Cancel
                  </Button>
                  <Button
                    type="submit"
                    disabled={
                      publishMutation.isPending ||
                      !registries ||
                      registries.length === 0 ||
                      !selectedRegistry ||
                      !repository.trim() ||
                      !tag.trim()
                    }
                  >
                    {publishMutation.isPending ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Publishing...
                      </>
                    ) : (
                      <>
                        <Upload className="mr-2 h-4 w-4" />
                        Publish
                      </>
                    )}
                  </Button>
                </div>
              </>
            )}
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
};
