import { useState, useEffect } from 'react';
import { useUpdateRegistry } from '@/hooks/useRegistries';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Loader2 } from 'lucide-react';
import type { OCIRegistry } from '@/types';

interface EditRegistryDialogProps {
  registry: OCIRegistry;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const EditRegistryDialog = ({ registry, open, onOpenChange }: EditRegistryDialogProps) => {
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [defaultRepository, setDefaultRepository] = useState('');
  const [isDefault, setIsDefault] = useState(false);
  const [error, setError] = useState('');

  const updateMutation = useUpdateRegistry();

  // Initialize form with registry data
  useEffect(() => {
    if (registry) {
      setName(registry.name);
      setUrl(registry.url);
      setUsername(registry.username || '');
      setPassword(''); // Don't pre-fill password for security
      setDefaultRepository(registry.default_repository || '');
      setIsDefault(registry.is_default);
    }
  }, [registry]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    try {
      await updateMutation.mutateAsync({
        id: registry.id,
        data: {
          name,
          url,
          username: username || undefined,
          password: password || undefined, // Only update if provided
          default_repository: defaultRepository || undefined,
          is_default: isDefault,
        },
      });
      onOpenChange(false);
      setError('');
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to update registry. Please try again.';
      setError(errorMessage);
      console.error('Failed to update registry:', err);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit Registry</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4 mt-4">
          <div className="space-y-2">
            <label className="text-sm font-medium">Name</label>
            <Input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g., GitHub Container Registry"
              required
              autoFocus
            />
          </div>

          <div className="space-y-2">
            <label className="text-sm font-medium">Registry URL</label>
            <Input
              type="text"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="e.g., ghcr.io or quay.io"
              required
            />
          </div>

          <div className="space-y-2">
            <label className="text-sm font-medium">
              Username <span className="text-muted-foreground">(optional)</span>
            </label>
            <Input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="Registry username"
            />
          </div>

          <div className="space-y-2">
            <label className="text-sm font-medium">
              Password/Token <span className="text-muted-foreground">(leave blank to keep current)</span>
            </label>
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Leave blank to keep current password"
            />
          </div>

          <div className="space-y-2">
            <label className="text-sm font-medium">
              Default Repository <span className="text-muted-foreground">(optional)</span>
            </label>
            <Input
              type="text"
              value={defaultRepository}
              onChange={(e) => setDefaultRepository(e.target.value)}
              placeholder="e.g., myorg/workspaces"
            />
            <p className="text-xs text-muted-foreground">
              Base path for repositories. Workspace name will be appended when publishing.
            </p>
          </div>

          {error && (
            <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-3 py-2 rounded text-sm">
              {error}
            </div>
          )}

          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="edit_is_default"
              checked={isDefault}
              onChange={(e) => setIsDefault(e.target.checked)}
              className="h-4 w-4 rounded border-input"
            />
            <label htmlFor="edit_is_default" className="text-sm font-medium cursor-pointer">
              Set as default registry
            </label>
          </div>

          <div className="flex gap-2 justify-end pt-4">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={updateMutation.isPending}>
              {updateMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Updating...
                </>
              ) : (
                'Update Registry'
              )}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
};
