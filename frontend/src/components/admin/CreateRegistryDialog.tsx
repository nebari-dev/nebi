import { useState } from 'react';
import { useCreateRegistry } from '@/hooks/useRegistries';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Plus, Loader2 } from 'lucide-react';

export const CreateRegistryDialog = () => {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [defaultRepository, setDefaultRepository] = useState('');
  const [isDefault, setIsDefault] = useState(false);
  const [error, setError] = useState('');

  const createMutation = useCreateRegistry();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    try {
      await createMutation.mutateAsync({
        name,
        url,
        username: username || undefined,
        password: password || undefined,
        default_repository: defaultRepository || undefined,
        is_default: isDefault,
      });
      setOpen(false);
      setName('');
      setUrl('');
      setUsername('');
      setPassword('');
      setDefaultRepository('');
      setIsDefault(false);
      setError('');
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to create registry. Please try again.';
      setError(errorMessage);
      console.error('Failed to create registry:', err);
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger>
        <Button onClick={() => setOpen(true)}>
          <Plus className="h-4 w-4 mr-2" />
          Add Registry
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add OCI Registry</DialogTitle>
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
              Password/Token <span className="text-muted-foreground">(optional)</span>
            </label>
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Personal access token or password"
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
              placeholder="e.g., myorg/environments"
            />
            <p className="text-xs text-muted-foreground">
              Base path for repositories. Environment name will be appended when publishing.
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
              id="is_default"
              checked={isDefault}
              onChange={(e) => setIsDefault(e.target.checked)}
              className="h-4 w-4 rounded border-input"
            />
            <label htmlFor="is_default" className="text-sm font-medium cursor-pointer">
              Set as default registry
            </label>
          </div>

          <div className="flex gap-2 justify-end pt-4">
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={createMutation.isPending}>
              {createMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Adding...
                </>
              ) : (
                'Add Registry'
              )}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
};
