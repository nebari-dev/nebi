import { ExternalLink, Loader2, Wifi, WifiOff } from 'lucide-react';
import { useCallback, useId, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import {
  useDisconnectServer,
  useRemoteConnect,
  useRemoteServer,
} from '@/hooks/useRemote';
import { openExternal } from '@/lib/openExternal';
import { useViewModeStore } from '@/store/viewModeStore';
import type { DeviceAuthorization } from '@/types';

export const Settings = () => {
  const { data: serverStatus, isLoading } = useRemoteServer();
  const disconnectMutation = useDisconnectServer();
  const setViewMode = useViewModeStore((s) => s.setViewMode);

  const [url, setUrl] = useState('');
  const [error, setError] = useState('');
  const urlId = useId();

  const handleConnected = useCallback(() => {
    setViewMode('remote'); // Auto-switch to remote view on successful connection
    setUrl('');
  }, [setViewMode]);
  const connect = useRemoteConnect({ onConnected: handleConnected });
  const connectState = connect.state;

  const handleConnect = (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    void connect.start(url);
  };

  const handleDisconnect = async () => {
    setError('');
    try {
      await disconnectMutation.mutateAsync();
      setViewMode('local'); // Switch back to local view on disconnect
    } catch (err: unknown) {
      const apiError = err as { response?: { data?: { error?: string } } };
      setError(
        apiError.response?.data?.error || 'Failed to disconnect from server',
      );
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
  const displayedError =
    error || (connectState.status === 'error' ? connectState.error : '');

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold flex items-center gap-3">Settings</h1>
        <p className="text-muted-foreground">
          Configure your local Nebi instance
        </p>
      </div>

      {displayedError && (
        <div
          role="alert"
          className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded"
        >
          {displayedError}
        </div>
      )}

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>Remote Server Connection</CardTitle>
            <Badge
              className={
                isConnected
                  ? 'bg-green-100 text-green-800 border-green-300'
                  : 'bg-zinc-100 text-zinc-800 border-zinc-300'
              }
            >
              {isConnected ? (
                <Wifi className="h-3 w-3 mr-1" />
              ) : (
                <WifiOff className="h-3 w-3 mr-1" />
              )}
              {isConnected ? 'Connected' : 'Disconnected'}
            </Badge>
          </div>
        </CardHeader>
        <CardContent>
          {isConnected ? (
            <div className="space-y-4">
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-muted-foreground w-24">
                    Server URL
                  </span>
                  <span className="text-sm font-mono">{serverStatus?.url}</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-muted-foreground w-24">
                    Username
                  </span>
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
          ) : connectState.status === 'pending' ? (
            <DeviceCodePrompt
              authorization={connectState.authorization}
              onCancel={connect.cancel}
            />
          ) : (
            <form onSubmit={handleConnect} className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Connect to a remote Nebi server to sync projects and access
                shared resources. You will approve the connection by signing in
                to the server in your browser.
              </p>
              <div className="space-y-2">
                <label htmlFor={urlId} className="text-sm font-medium">
                  Server URL
                </label>
                <Input
                  id={urlId}
                  type="url"
                  placeholder="https://nebi.example.com"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  required
                />
              </div>
              <Button
                render={<button type="submit" />}
                disabled={connectState.status === 'starting'}
              >
                {connectState.status === 'starting' ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Connecting...
                  </>
                ) : (
                  'Connect'
                )}
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
};

type DeviceCodePromptProps = {
  authorization: DeviceAuthorization;
  onCancel: () => void;
};

const DeviceCodePrompt = ({
  authorization,
  onCancel,
}: DeviceCodePromptProps) => {
  const signInUrl =
    authorization.verification_uri_complete || authorization.verification_uri;

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        Open the sign-in page, sign in to the remote server, and confirm this
        code:
      </p>
      <p className="font-mono text-3xl font-semibold tracking-widest">
        {authorization.user_code}
      </p>
      <p className="text-sm text-muted-foreground">
        Sign-in page:{' '}
        <span className="font-mono break-all">
          {authorization.verification_uri}
        </span>
      </p>
      <div className="flex flex-wrap items-center gap-3">
        <Button onClick={() => openExternal(signInUrl)}>
          <ExternalLink className="mr-2 h-4 w-4" />
          Open sign-in page
        </Button>
        <Button variant="outline" onClick={onCancel}>
          Cancel
        </Button>
      </div>
      <p
        role="status"
        className="flex items-center gap-2 text-sm text-muted-foreground"
      >
        <Loader2 className="h-4 w-4 animate-spin" />
        Waiting for approval...
      </p>
    </div>
  );
};
