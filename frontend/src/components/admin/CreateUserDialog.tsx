import { Loader2, Plus } from 'lucide-react';
import { useEffect, useId, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { useCreateUser } from '@/hooks/useAdmin';

export const CreateUserDialog = () => {
  const [open, setOpen] = useState(false);
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [isAdmin, setIsAdmin] = useState(false);
  const [error, setError] = useState('');
  const usernameId = useId();
  const emailId = useId();
  const passwordId = useId();
  const confirmPasswordId = useId();
  const isAdminId = useId();
  const usernameInputRef = useRef<HTMLInputElement>(null);

  const createMutation = useCreateUser();

  useEffect(() => {
    if (open) {
      usernameInputRef.current?.focus();
    }
  }, [open]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    if (password !== confirmPassword) {
      setError('Passwords do not match');
      return;
    }

    try {
      await createMutation.mutateAsync({
        username,
        email,
        password,
        is_admin: isAdmin,
      });
      setOpen(false);
      setUsername('');
      setEmail('');
      setPassword('');
      setConfirmPassword('');
      setIsAdmin(false);
      setError('');
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage =
        error?.response?.data?.error ||
        'Failed to create user. Please try again.';
      setError(errorMessage);
      console.error('Failed to create user:', err);
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger
        render={
          <Button onClick={() => setOpen(true)}>
            <Plus className="h-4 w-4 mr-2" />
            Create User
          </Button>
        }
      />
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create New User</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4 mt-4">
          <div className="space-y-2">
            <label htmlFor={usernameId} className="text-sm font-medium">
              Username
            </label>
            <Input
              ref={usernameInputRef}
              id={usernameId}
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
            />
          </div>

          <div className="space-y-2">
            <label htmlFor={emailId} className="text-sm font-medium">
              Email
            </label>
            <Input
              id={emailId}
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
            />
          </div>

          <div className="space-y-2">
            <label htmlFor={passwordId} className="text-sm font-medium">
              Password
            </label>
            <Input
              id={passwordId}
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>

          <div className="space-y-2">
            <label htmlFor={confirmPasswordId} className="text-sm font-medium">
              Confirm Password
            </label>
            <Input
              id={confirmPasswordId}
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
            />
            {confirmPassword && password !== confirmPassword && (
              <p className="text-xs text-red-500">Passwords do not match</p>
            )}
          </div>

          {error && (
            <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-3 py-2 rounded text-sm">
              {error}
            </div>
          )}

          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id={isAdminId}
              checked={isAdmin}
              onChange={(e) => setIsAdmin(e.target.checked)}
              className="h-4 w-4 rounded border-input"
            />
            <label
              htmlFor={isAdminId}
              className="text-sm font-medium cursor-pointer"
            >
              Make Admin
            </label>
          </div>

          <div className="flex gap-2 justify-end pt-4">
            <Button
              type="button"
              variant="outline"
              onClick={() => setOpen(false)}
            >
              Cancel
            </Button>
            <Button
              render={<button type="submit" />}
              disabled={createMutation.isPending}
            >
              {createMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Creating...
                </>
              ) : (
                'Create User'
              )}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
};
