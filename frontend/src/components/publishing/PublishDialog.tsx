import { useState, useEffect, useMemo } from 'react';
import { usePublicRegistries, usePublishWorkspace, usePublications } from '@/hooks/useRegistries';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Loader2, Upload, AlertCircle } from 'lucide-react';

interface PublishDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  environmentId: string;
  environmentName: string;
}

// Helper function to suggest next version tag
const suggestNextTag = (existingTags: string[]): string => {
  if (existingTags.length === 0) {
    return 'v1';
  }

  // Extract version numbers from tags like "v1", "v2", "v10", etc.
  const versionNumbers = existingTags
    .map(tag => {
      const match = tag.match(/^v(\d+)$/);
      return match ? parseInt(match[1], 10) : null;
    })
    .filter((n): n is number => n !== null);

  if (versionNumbers.length === 0) {
    return 'v1';
  }

  const maxVersion = Math.max(...versionNumbers);
  return `v${maxVersion + 1}`;
};

export const PublishDialog = ({ open, onOpenChange, environmentId, environmentName }: PublishDialogProps) => {
  const { data: registries, isLoading: registriesLoading } = usePublicRegistries();
  const { data: publications, isLoading: publicationsLoading } = usePublications(environmentId);
  const publishMutation = usePublishWorkspace();

  const [selectedRegistry, setSelectedRegistry] = useState('');
  const [repository, setRepository] = useState('');
  const [tag, setTag] = useState('');
  const [error, setError] = useState('');
  const [publishSuccess, setPublishSuccess] = useState(false);
  const [hasAutoPopulated, setHasAutoPopulated] = useState(false);

  // Find default registry
  const defaultRegistry = useMemo(() => {
    return registries?.find(r => r.is_default);
  }, [registries]);

  // Get selected registry object
  const selectedRegistryObj = useMemo(() => {
    return registries?.find(r => r.id === selectedRegistry);
  }, [registries, selectedRegistry]);

  // Auto-populate all fields when dialog opens and data is loaded
  useEffect(() => {
    if (open && !hasAutoPopulated && registries && publications) {
      // Auto-select default registry
      if (defaultRegistry) {
        setSelectedRegistry(defaultRegistry.id);

        // Auto-populate repository based on default registry's default_repository
        // Append first 8 chars of workspace ID to avoid collisions
        const envIdSuffix = environmentId.slice(0, 8);
        if (defaultRegistry.default_repository) {
          const baseRepo = defaultRegistry.default_repository.replace(/\/$/, '');
          setRepository(`${baseRepo}/${environmentName}-${envIdSuffix}`);
        } else {
          setRepository(`${environmentName}-${envIdSuffix}`);
        }
      }

      // Auto-populate tag based on existing publications
      const existingTagNames = publications.map(p => p.tag);
      setTag(suggestNextTag(existingTagNames));

      setHasAutoPopulated(true);
    }
  }, [open, hasAutoPopulated, registries, publications, defaultRegistry, environmentName]);

  // Update repository when registry selection changes (after initial auto-populate)
  useEffect(() => {
    if (hasAutoPopulated && selectedRegistryObj) {
      const envIdSuffix = environmentId.slice(0, 8);
      if (selectedRegistryObj.default_repository) {
        const baseRepo = selectedRegistryObj.default_repository.replace(/\/$/, '');
        setRepository(`${baseRepo}/${environmentName}-${envIdSuffix}`);
      } else {
        setRepository(`${environmentName}-${envIdSuffix}`);
      }
    }
  }, [selectedRegistryObj, hasAutoPopulated, environmentName, environmentId]);

  // Reset form when dialog closes
  useEffect(() => {
    if (!open) {
      setSelectedRegistry('');
      setRepository('');
      setTag('');
      setError('');
      setPublishSuccess(false);
      setHasAutoPopulated(false);
    }
  }, [open]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    if (!selectedRegistry || !repository.trim() || !tag.trim()) {
      setError('All fields are required');
      return;
    }

    try {
      await publishMutation.mutateAsync({
        workspaceId: environmentId,
        data: {
          registry_id: selectedRegistry,
          repository: repository.trim(),
          tag: tag.trim(),
        },
      });
      setPublishSuccess(true);
      setTimeout(() => {
        onOpenChange(false);
        // Refresh the page to show the new publication in the Publications tab
        window.location.reload();
      }, 2000);
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to publish workspace. Please try again.';
      setError(errorMessage);
      console.error('Failed to publish:', err);
    }
  };

  const handleClose = () => {
    if (!publishMutation.isPending) {
      onOpenChange(false);
    }
  };

  const isLoading = registriesLoading || publicationsLoading;

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Publish Workspace to OCI Registry</DialogTitle>
          <DialogDescription>
            Publish the workspace's pixi.toml and pixi.lock files as an OCI artifact.
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
            {isLoading ? (
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
                        {registry.is_default ? ' (Default)' : ''}
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
                    {publications && publications.length > 0 && (
                      <> (existing: {publications.slice(0, 3).map(p => p.tag).join(', ')}{publications.length > 3 ? '...' : ''})</>
                    )}
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
