import { useState } from 'react';
import { useRemoteServer, useAutoConnect, useDisconnectServer } from '@/hooks/useRemote';
import { useViewModeStore } from '@/store/viewModeStore';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Settings as SettingsIcon, Loader2, Wifi, WifiOff, ExternalLink } from 'lucide-react';

export const Settings = () => {
  const { data: serverStatus, isLoading } = useRemoteServer();
  const disconnectMutation = useDisconnectServer();
  const setViewMode = useViewModeStore((s) => s.setViewMode);
  const { approvalUrl, connecting, error: connectError, startDeviceCodeFlow, config } = useAutoConnect();

  const [url, setUrl] = useState('');
  const [error, setError] = useState('');

  const handleConnect = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    const serverUrl = url || config?.remote_url;
    if (!serverUrl) {
      setError('Please enter a server URL');
      return;
    }
    startDeviceCodeFlow(serverUrl);
  };

  const handleDisconnect = async () => {
    setError('');
    try {
      await disconnectMutation.mutateAsync();
      setViewMode('local');
    } catch (err: unknown) {
      const apiError = err as { response?: { data?: { error?: string } } };
      setError(apiError.response?.data?.error || 'Failed to disconnect from server');
    }
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  const isConnected = serverStatus?.status === 'connected';
  const displayError = error || connectError;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold flex items-center gap-3">
          <SettingsIcon className="h-8 w-8" />
          Settings
        </h1>
        <p className="text-muted-foreground">Configure your local Nebi instance</p>
      </div>

      {displayError && (
        <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded">
          {displayError}
        </div>
      )}

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>Remote Server Connection</CardTitle>
            <Badge
              className={
                isConnected
                  ? 'bg-green-500/10 text-green-500 border-green-500/20'
                  : connecting
                    ? 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20'
                    : 'bg-gray-500/10 text-gray-500 border-gray-500/20'
              }
            >
              {isConnected ? (
                <Wifi className="h-3 w-3 mr-1" />
              ) : connecting ? (
                <Loader2 className="h-3 w-3 mr-1 animate-spin" />
              ) : (
                <WifiOff className="h-3 w-3 mr-1" />
              )}
              {isConnected ? 'Connected' : connecting ? 'Connecting...' : 'Disconnected'}
            </Badge>
          </div>
        </CardHeader>
        <CardContent>
          {isConnected ? (
            <div className="space-y-4">
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-muted-foreground w-24">Server URL</span>
                  <span className="text-sm font-mono">{serverStatus?.url}</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-muted-foreground w-24">Username</span>
                  <span className="text-sm">{serverStatus?.username}</span>
                </div>
              </div>
              <div className="pt-2">
                <Button
                  variant="destructive"
                  onClick={handleDisconnect}
                  disabled={disconnectMutation.isPending}
                >
                  {disconnectMutation.isPending ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Disconnecting...
                    </>
                  ) : (
                    'Disconnect'
                  )}
                </Button>
              </div>
            </div>
          ) : connecting ? (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Waiting for you to sign in. A browser window should have opened automatically.
              </p>
              {approvalUrl && (
                <div className="space-y-2">
                  <p className="text-sm text-muted-foreground">
                    If it didn't open, click the link below:
                  </p>
                  <a
                    href={approvalUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1.5 text-sm text-primary hover:underline"
                  >
                    <ExternalLink className="h-3.5 w-3.5" />
                    Open sign-in page
                  </a>
                </div>
              )}
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                Polling for approval...
              </div>
            </div>
          ) : (
            <form onSubmit={handleConnect} className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Connect to a remote Nebi server to sync workspaces and access shared resources.
              </p>
              <div className="space-y-2">
                <label className="text-sm font-medium">Server URL</label>
                <Input
                  type="url"
                  placeholder="https://nebi.example.com"
                  value={url || config?.remote_url || ''}
                  onChange={(e) => setUrl(e.target.value)}
                  required
                />
              </div>
              <Button type="submit">
                Connect
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
};
