import {
  KeyRound,
  Loader2,
  Pencil,
  Plus,
  Save,
  Trash2,
  Wifi,
  WifiOff,
  X,
} from 'lucide-react';
import { useId, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Input } from '@/components/ui/input';
import {
  useBuildEnvVars,
  useDeleteBuildEnvVar,
  useUpsertBuildEnvVar,
} from '@/hooks/useBuildEnv';
import {
  useConnectServer,
  useDisconnectServer,
  useRemoteServer,
} from '@/hooks/useRemote';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';

const buildEnvDateFormatter = new Intl.DateTimeFormat(undefined, {
  dateStyle: 'medium',
  timeStyle: 'medium',
});

const formatBuildEnvTimestamp = (value: string) => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return buildEnvDateFormatter.format(date);
};

const getBuildEnvTimestampLabel = (createdAt: string, updatedAt: string) => {
  const createdDate = new Date(createdAt);
  const updatedDate = new Date(updatedAt);
  if (
    !Number.isNaN(createdDate.getTime()) &&
    !Number.isNaN(updatedDate.getTime()) &&
    createdDate.getTime() === updatedDate.getTime()
  ) {
    return `Created ${formatBuildEnvTimestamp(createdAt)}`;
  }

  return `Updated ${formatBuildEnvTimestamp(updatedAt || createdAt)}`;
};

const getApiError = (err: unknown, fallback: string) => {
  const apiError = err as { response?: { data?: { error?: string } } };
  return apiError.response?.data?.error || fallback;
};

const LocalRemoteConnectionSettings = () => {
  const { data: serverStatus, isLoading } = useRemoteServer();
  const connectMutation = useConnectServer();
  const disconnectMutation = useDisconnectServer();
  const setViewMode = useViewModeStore((s) => s.setViewMode);

  const [url, setUrl] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const urlId = useId();
  const usernameId = useId();
  const passwordId = useId();

  const handleConnect = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    try {
      await connectMutation.mutateAsync({ url, username, password });
      setViewMode('remote'); // Auto-switch to remote view on successful connection
      setUrl('');
      setUsername('');
      setPassword('');
    } catch (err: unknown) {
      setError(getApiError(err, 'Failed to connect to server'));
    }
  };

  const handleDisconnect = async () => {
    setError('');
    try {
      await disconnectMutation.mutateAsync();
      setViewMode('local'); // Switch back to local view on disconnect
    } catch (err: unknown) {
      setError(getApiError(err, 'Failed to disconnect from server'));
    }
  };

  const isConnected = serverStatus?.status === 'connected';

  return (
    <>
      {error && (
        <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded">
          {error}
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
          {isLoading ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading connection
            </div>
          ) : isConnected ? (
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
          ) : (
            <form onSubmit={handleConnect} className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Connect to a remote Nebi server to sync workspaces and access
                shared resources.
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
              <div className="space-y-2">
                <label htmlFor={usernameId} className="text-sm font-medium">
                  Username
                </label>
                <Input
                  id={usernameId}
                  type="text"
                  placeholder="Username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
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
                  placeholder="Password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                />
              </div>
              <Button type="submit" disabled={connectMutation.isPending}>
                {connectMutation.isPending ? (
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
    </>
  );
};

export const Settings = () => {
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const viewMode = useViewModeStore((s) => s.viewMode);
  const { data: serverStatus } = useRemoteServer(isLocalMode);
  const isRemoteConnected = isLocalMode && serverStatus?.status === 'connected';
  const buildEnvTarget =
    isRemoteConnected && viewMode === 'remote' ? 'remote' : 'local';
  const { data: buildEnvVars, isLoading: buildEnvLoading } =
    useBuildEnvVars(buildEnvTarget);
  const upsertBuildEnvVar = useUpsertBuildEnvVar(buildEnvTarget);
  const deleteBuildEnvVar = useDeleteBuildEnvVar(buildEnvTarget);

  const [buildEnvError, setBuildEnvError] = useState('');
  const [buildEnvKey, setBuildEnvKey] = useState('');
  const [buildEnvValue, setBuildEnvValue] = useState('');
  const [editingBuildEnvKey, setEditingBuildEnvKey] = useState<string | null>(
    null,
  );
  const [editingBuildEnvValue, setEditingBuildEnvValue] = useState('');
  const [deleteBuildEnvKey, setDeleteBuildEnvKey] = useState<string | null>(
    null,
  );
  const buildEnvKeyId = useId();
  const buildEnvValueId = useId();

  const clearBuildEnvEdit = () => {
    setEditingBuildEnvKey(null);
    setEditingBuildEnvValue('');
  };

  const handleSaveBuildEnvVar = async (e: React.FormEvent) => {
    e.preventDefault();
    setBuildEnvError('');
    try {
      await upsertBuildEnvVar.mutateAsync({
        key: buildEnvKey,
        value: buildEnvValue,
      });
      setBuildEnvKey('');
      setBuildEnvValue('');
      clearBuildEnvEdit();
    } catch (err: unknown) {
      setBuildEnvError(getApiError(err, 'Failed to save build variable'));
    }
  };

  const handleUpdateBuildEnvVar = async (e: React.FormEvent, key: string) => {
    e.preventDefault();
    setBuildEnvError('');
    try {
      await upsertBuildEnvVar.mutateAsync({
        key,
        value: editingBuildEnvValue,
      });
      clearBuildEnvEdit();
    } catch (err: unknown) {
      setBuildEnvError(getApiError(err, 'Failed to update build variable'));
    }
  };

  const handleDeleteBuildEnvVar = async () => {
    if (!deleteBuildEnvKey) {
      return;
    }
    setBuildEnvError('');
    try {
      await deleteBuildEnvVar.mutateAsync(deleteBuildEnvKey);
      if (buildEnvKey === deleteBuildEnvKey) {
        setBuildEnvKey('');
        setBuildEnvValue('');
      }
      if (editingBuildEnvKey === deleteBuildEnvKey) {
        clearBuildEnvEdit();
      }
    } catch (err: unknown) {
      setBuildEnvError(getApiError(err, 'Failed to delete build variable'));
    } finally {
      setDeleteBuildEnvKey(null);
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold flex items-center gap-3">Settings</h1>
        <p className="text-muted-foreground">
          {isLocalMode
            ? 'Configure your local Nebi instance'
            : 'Manage your Nebi preferences'}
        </p>
      </div>

      {isLocalMode && <LocalRemoteConnectionSettings />}

      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <CardTitle className="flex items-center gap-2">
              <KeyRound className="h-5 w-5" />
              Build Variables
            </CardTitle>
            <div className="flex items-center gap-2">
              <Badge variant="secondary">
                {buildEnvTarget === 'remote' ? 'Server' : 'Local'}
              </Badge>
              <Badge variant="outline">
                {buildEnvVars?.length || 0} configured
              </Badge>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {buildEnvError && (
            <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded">
              {buildEnvError}
            </div>
          )}

          <form
            onSubmit={handleSaveBuildEnvVar}
            className="grid gap-3 md:grid-cols-[minmax(180px,0.7fr)_1fr_auto] md:items-end"
          >
            <div className="space-y-2">
              <label htmlFor={buildEnvKeyId} className="text-sm font-medium">
                Name
              </label>
              <Input
                id={buildEnvKeyId}
                value={buildEnvKey}
                onChange={(e) => setBuildEnvKey(e.target.value)}
                placeholder="GITLAB_TOKEN"
                autoCapitalize="off"
                autoCorrect="off"
                spellCheck={false}
                required
              />
            </div>
            <div className="space-y-2">
              <label htmlFor={buildEnvValueId} className="text-sm font-medium">
                Value
              </label>
              <Input
                id={buildEnvValueId}
                type="password"
                value={buildEnvValue}
                onChange={(e) => setBuildEnvValue(e.target.value)}
                placeholder="********"
                required
              />
            </div>
            <Button
              type="submit"
              className="gap-2"
              disabled={upsertBuildEnvVar.isPending}
            >
              {upsertBuildEnvVar.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Plus className="h-4 w-4" />
              )}
              Add
            </Button>
          </form>

          <div className="rounded-md border divide-y">
            {buildEnvLoading ? (
              <div className="flex items-center gap-2 p-4 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                Loading variables
              </div>
            ) : buildEnvVars && buildEnvVars.length > 0 ? (
              buildEnvVars.map((envVar) => {
                const isEditing = editingBuildEnvKey === envVar.key;

                return (
                  <div
                    key={envVar.id}
                    className="grid gap-3 p-3 md:grid-cols-[1fr_auto_auto] md:items-center"
                  >
                    <div className="min-w-0">
                      <div className="font-mono text-sm truncate">
                        {envVar.key}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        {getBuildEnvTimestampLabel(
                          envVar.created_at,
                          envVar.updated_at,
                        )}
                      </div>
                    </div>
                    {isEditing ? (
                      <form
                        onSubmit={(e) => handleUpdateBuildEnvVar(e, envVar.key)}
                        className="grid gap-2 md:col-span-2 md:grid-cols-[minmax(180px,1fr)_auto_auto] md:items-center"
                      >
                        <Input
                          type="password"
                          value={editingBuildEnvValue}
                          onChange={(e) =>
                            setEditingBuildEnvValue(e.target.value)
                          }
                          placeholder="New value"
                          aria-label={`New value for ${envVar.key}`}
                          required
                          autoFocus
                        />
                        <Button
                          type="submit"
                          size="sm"
                          className="gap-2"
                          disabled={upsertBuildEnvVar.isPending}
                        >
                          {upsertBuildEnvVar.isPending ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            <Save className="h-4 w-4" />
                          )}
                          Save
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="gap-2"
                          onClick={clearBuildEnvEdit}
                        >
                          <X className="h-4 w-4" />
                          Cancel
                        </Button>
                      </form>
                    ) : (
                      <>
                        <Badge variant="outline">Encrypted</Badge>
                        <div className="flex items-center gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            title="Edit value"
                            aria-label={`Edit ${envVar.key}`}
                            onClick={() => {
                              setBuildEnvError('');
                              setEditingBuildEnvKey(envVar.key);
                              setEditingBuildEnvValue('');
                            }}
                          >
                            <Pencil className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="text-red-600 hover:text-red-700"
                            title="Delete variable"
                            aria-label={`Delete ${envVar.key}`}
                            onClick={() => setDeleteBuildEnvKey(envVar.key)}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </>
                    )}
                  </div>
                );
              })
            ) : (
              <div className="p-4 text-sm text-muted-foreground">
                No build variables configured
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      <ConfirmDialog
        open={deleteBuildEnvKey !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteBuildEnvKey(null);
          }
        }}
        onConfirm={handleDeleteBuildEnvVar}
        title="Delete build variable?"
        description={`This removes ${deleteBuildEnvKey || 'this variable'} from future builds.`}
        confirmText="Delete"
        variant="destructive"
      />
    </div>
  );
};
