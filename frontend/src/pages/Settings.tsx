import { useState } from 'react';
import { useRemoteServer, useConnectServer, useDisconnectServer } from '@/hooks/useRemote';
import { useRemoteStore } from '@/store/remoteStore';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Loader2, Plug, Unplug, Server } from 'lucide-react';

export const Settings = () => {
  const { data: serverStatus, isLoading } = useRemoteServer();
  const connectMutation = useConnectServer();
  const disconnectMutation = useDisconnectServer();
  const { setConnection, clearConnection } = useRemoteStore();

  const [url, setUrl] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');

  const isConnected = serverStatus?.status === 'connected';

  const handleConnect = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!url.trim() || !username.trim() || !password.trim()) return;

    setError('');
    try {
      const result = await connectMutation.mutateAsync({ url, username, password });
      setConnection(result.url, result.username);
      setPassword('');
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      setError(error?.response?.data?.error || 'Failed to connect to remote server');
    }
  };

  const handleDisconnect = async () => {
    setError('');
    try {
      await disconnectMutation.mutateAsync();
      clearConnection();
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      setError(error?.response?.data?.error || 'Failed to disconnect');
    }
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold">Settings</h1>
        <p className="text-muted-foreground">Configure your desktop app</p>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <Server className="h-5 w-5" />
              <CardTitle>Remote Server</CardTitle>
            </div>
            <Badge
              className={
                isConnected
                  ? 'bg-green-500/10 text-green-500 border-green-500/20'
                  : 'bg-zinc-500/10 text-zinc-500 border-zinc-500/20'
              }
            >
              {isConnected ? 'Connected' : 'Disconnected'}
            </Badge>
          </div>
        </CardHeader>
        <CardContent>
          {isConnected ? (
            <div className="space-y-4">
              <div className="grid gap-3">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Server URL:</span>
                  <span className="font-mono text-sm">{serverStatus?.url}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Username:</span>
                  <span className="font-medium">{serverStatus?.username}</span>
                </div>
              </div>
              <Button
                variant="destructive"
                onClick={handleDisconnect}
                disabled={disconnectMutation.isPending}
                className="gap-2"
              >
                {disconnectMutation.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Unplug className="h-4 w-4" />
                )}
                Disconnect
              </Button>
            </div>
          ) : (
            <form onSubmit={handleConnect} className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Connect to a remote Nebi server to browse its workspaces.
              </p>
              <div className="space-y-2">
                <label className="text-sm font-medium">Server URL</label>
                <Input
                  placeholder="https://nebi.example.com"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">Username</label>
                <Input
                  placeholder="admin"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">Password</label>
                <Input
                  type="password"
                  placeholder="Password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                />
              </div>
              <Button
                type="submit"
                disabled={connectMutation.isPending}
                className="gap-2"
              >
                {connectMutation.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Plug className="h-4 w-4" />
                )}
                Connect
              </Button>
            </form>
          )}

          {error && (
            <div className="mt-4 bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded">
              {error}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
};
