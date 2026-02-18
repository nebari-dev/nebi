import { useState, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useWorkspaces, useCreateWorkspace, useDeleteWorkspace } from '@/hooks/useWorkspaces';
import { useRemoteServer, useRemoteWorkspaces, useCreateRemoteWorkspace, useDeleteRemoteWorkspace } from '@/hooks/useRemote';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';
import { workspacesApi } from '@/api/workspaces';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { PixiTomlEditor } from '@/components/workspace/PixiTomlEditor';
import { Loader2, Plus, Trash2, X, Edit } from 'lucide-react';

type UnifiedWorkspace = {
  id: string;
  name: string;
  status: string;
  package_manager: string;
  created_at: string;
  location: 'local' | 'remote';
  // Local-only fields
  source?: 'local' | 'managed';
  path?: string;
  owner_id?: string;
  owner?: { id: string; username: string; email: string };
  size_formatted?: string;
};

const statusColors: Record<string, string> = {
  pending: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
  creating: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  ready: 'bg-green-500/10 text-green-500 border-green-500/20',
  failed: 'bg-red-500/10 text-red-500 border-red-500/20',
  deleting: 'bg-orange-500/10 text-orange-500 border-orange-500/20',
};

const DEFAULT_PIXI_TOML = `[workspace]
name = "my-project"
channels = ["conda-forge"]
platforms = ["osx-arm64", "linux-64"]

[dependencies]
python = ">=3.11"
`;

export const Workspaces = () => {
  const navigate = useNavigate();
  const { data: workspaces, isLoading } = useWorkspaces();
  const createMutation = useCreateWorkspace();
  const deleteMutation = useDeleteWorkspace();
  const createRemoteMutation = useCreateRemoteWorkspace();
  const deleteRemoteMutation = useDeleteRemoteWorkspace();
  const isLocal = useModeStore((state) => state.mode === 'local');
  const { data: serverStatus } = useRemoteServer();
  const isRemoteConnected = isLocal && serverStatus?.status === 'connected';
  const { data: remoteWorkspaces, isLoading: remoteLoading } = useRemoteWorkspaces(isRemoteConnected);
  const viewMode = useViewModeStore((state) => state.viewMode);

  const [showCreate, setShowCreate] = useState(false);
  const [createTarget, setCreateTarget] = useState<'local' | 'server'>('local');
  const [newWsName, setNewWsName] = useState('');
  const [localPath, setLocalPath] = useState('');
  const [pixiToml, setPixiToml] = useState(DEFAULT_PIXI_TOML);

  const [showEdit, setShowEdit] = useState(false);
  const [editWsId, setEditWsId] = useState<string | null>(null);
  const [editWsName, setEditWsName] = useState('');
  const [editPixiToml, setEditPixiToml] = useState('');
  const [loadingEdit, setLoadingEdit] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<{ id: string; name: string; location: 'local' | 'remote' } | null>(null);
  const [error, setError] = useState('');

  // Filter workspaces based on view mode (when remote connected) or show all local (when not)
  const displayedWorkspaces = useMemo<UnifiedWorkspace[]>(() => {
    // If not connected to remote, always show local workspaces
    if (!isRemoteConnected) {
      if (!workspaces) return [];
      return workspaces.map((ws) => ({
        id: ws.id,
        name: ws.name,
        status: ws.status,
        package_manager: ws.package_manager,
        created_at: ws.created_at,
        location: 'local' as const,
        source: ws.source,
        path: ws.path,
        owner_id: ws.owner_id,
        owner: ws.owner,
        size_formatted: ws.size_formatted,
      }));
    }

    // When connected, show based on viewMode
    if (viewMode === 'local') {
      if (!workspaces) return [];
      return workspaces.map((ws) => ({
        id: ws.id,
        name: ws.name,
        status: ws.status,
        package_manager: ws.package_manager,
        created_at: ws.created_at,
        location: 'local' as const,
        source: ws.source,
        path: ws.path,
        owner_id: ws.owner_id,
        owner: ws.owner,
        size_formatted: ws.size_formatted,
      }));
    } else {
      if (!remoteWorkspaces) return [];
      return remoteWorkspaces.map((ws) => ({
        id: ws.id,
        name: ws.name,
        status: ws.status,
        package_manager: ws.package_manager,
        created_at: ws.created_at,
        location: 'remote' as const,
        owner: ws.owner,
      }));
    }
  }, [workspaces, remoteWorkspaces, isRemoteConnected, viewMode]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newWsName.trim()) return;

    setError('');
    try {
      const tomlContent = pixiToml;

      if (createTarget === 'server' && isRemoteConnected) {
        await createRemoteMutation.mutateAsync({
          name: newWsName,
          package_manager: 'pixi',
          pixi_toml: tomlContent,
        });
      } else {
        await createMutation.mutateAsync({
          name: newWsName,
          package_manager: 'pixi',
          pixi_toml: tomlContent,
          ...(localPath.trim() ? { path: localPath.trim(), source: 'local' as const } : {}),
        });
      }

      // Reset form
      setNewWsName('');
      setLocalPath('');
      setPixiToml(DEFAULT_PIXI_TOML);
      setShowCreate(false);

      if (createTarget === 'local' || !isRemoteConnected) {
        navigate('/jobs');
      }
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to create workspace. Please try again.';
      setError(errorMessage);
    }
  };

  const handleDelete = async () => {
    if (!confirmDelete) return;

    setError('');
    try {
      if (confirmDelete.location === 'remote') {
        await deleteRemoteMutation.mutateAsync(confirmDelete.id);
      } else {
        await deleteMutation.mutateAsync(confirmDelete.id);
      }
      setConfirmDelete(null);
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to delete workspace. Please try again.';
      setError(errorMessage);
      setConfirmDelete(null);
    }
  };

  const handleEdit = async (id: string, name: string) => {
    setLoadingEdit(true);
    setError('');
    try {
      const { content } = await workspacesApi.getPixiToml(id);
      setEditWsId(id);
      setEditWsName(name);
      setEditPixiToml(content);
      setShowEdit(true);
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to load pixi.toml content. Please try again.';
      setError(errorMessage);
    } finally {
      setLoadingEdit(false);
    }
  };

  const handleEditSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editWsId || !editWsName.trim()) return;

    setError('');
    try {
      await deleteMutation.mutateAsync(editWsId);

      await createMutation.mutateAsync({
        name: editWsName,
        package_manager: 'pixi',
        pixi_toml: editPixiToml
      });

      setShowEdit(false);
      setEditWsId(null);
      setEditWsName('');
      setEditPixiToml('');

      navigate('/jobs');
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to update workspace. Please try again.';
      setError(errorMessage);
    }
  };

  const isCreatePending = createMutation.isPending || createRemoteMutation.isPending;
  const isDeletePending = deleteMutation.isPending || deleteRemoteMutation.isPending;

  if (isLoading || (isRemoteConnected && remoteLoading)) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold">Workspaces</h1>
          <p className="text-muted-foreground">Manage your development workspaces</p>
        </div>
        <Button onClick={() => {
          setShowCreate(!showCreate);
          setCreateTarget(isRemoteConnected && viewMode === 'remote' ? 'server' : 'local');
          setError('');
        }}>
          <Plus className="h-4 w-4 mr-2" />
          New Workspace
        </Button>
      </div>

      {error && (
        <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded">
          {error}
        </div>
      )}

      {showCreate && (
        <Card>
          <CardHeader>
            <div className="flex justify-between items-center">
              <CardTitle>Create New Workspace</CardTitle>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setShowCreate(false)}
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleCreate} className="space-y-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">Workspace Name</label>
                <Input
                  placeholder="e.g., my-data-project"
                  value={newWsName}
                  onChange={(e) => setNewWsName(e.target.value)}
                  autoFocus
                  required
                />
              </div>

              {/* Path field — only for local target in local mode */}
              {createTarget === 'local' && isLocal && (
                <div className="space-y-2">
                  <label className="text-sm font-medium">Path (optional)</label>
                  <Input
                    placeholder="e.g., /home/user/projects/my-project"
                    value={localPath}
                    onChange={(e) => setLocalPath(e.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">
                    Specify a local directory path for this workspace.
                  </p>
                </div>
              )}

              <PixiTomlEditor tomlValue={pixiToml} onTomlChange={setPixiToml} workspaceName={newWsName || 'my-project'} />

              <div className="flex gap-2 justify-end">
                <Button type="button" variant="outline" onClick={() => setShowCreate(false)}>
                  Cancel
                </Button>
                <Button type="submit" disabled={isCreatePending}>
                  {isCreatePending ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Creating...
                    </>
                  ) : (
                    createTarget === 'server' && isRemoteConnected
                      ? 'Create on Server'
                      : 'Create Workspace'
                  )}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      {showEdit && (
        <Card>
          <CardHeader>
            <div className="flex justify-between items-center">
              <CardTitle>Edit Workspace - {editWsName}</CardTitle>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setShowEdit(false)}
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleEditSubmit} className="space-y-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">Workspace Name</label>
                <Input
                  placeholder="e.g., my-data-project"
                  value={editWsName}
                  onChange={(e) => setEditWsName(e.target.value)}
                  required
                />
              </div>

              <PixiTomlEditor tomlValue={editPixiToml} onTomlChange={setEditPixiToml} workspaceName={editWsName || 'my-project'} />

              <div className="flex gap-2 justify-end">
                <Button type="button" variant="outline" onClick={() => setShowEdit(false)}>
                  Cancel
                </Button>
                <Button type="submit" disabled={createMutation.isPending || deleteMutation.isPending}>
                  {createMutation.isPending || deleteMutation.isPending ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Recreating...
                    </>
                  ) : (
                    'Save & Recreate'
                  )}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="border-b bg-muted/50">
                <tr>
                  <th className="text-left p-4 font-medium">Name</th>
                  <th className="text-left p-4 font-medium">Status</th>
                  <th className="text-left p-4 font-medium">Package Manager</th>
                  <th className="text-left p-4 font-medium">Size</th>
                  <th className="text-left p-4 font-medium">Created</th>
                  <th className="text-right p-4 font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {displayedWorkspaces.map((ws) => (
                  <tr
                    key={`${ws.location}-${ws.id}`}
                    className="border-b last:border-0 hover:bg-muted/50 cursor-pointer transition-colors"
                    onClick={() =>
                      ws.location === 'remote'
                        ? navigate(`/remote/workspaces/${ws.id}`)
                        : navigate(`/workspaces/${ws.id}`)
                    }
                  >
                    <td className="p-4 font-medium">
                      <div className="flex items-center gap-2">
                        {ws.name}
                      </div>
                      {ws.location === 'local' && ws.path && (
                        <div className="text-xs text-muted-foreground font-normal mt-0.5 font-mono truncate max-w-xs" title={ws.path}>
                          {ws.path}
                        </div>
                      )}
                    </td>
                    <td className="p-4">
                      <Badge className={statusColors[ws.status] || 'bg-zinc-500/10 text-zinc-500 border-zinc-500/20'}>
                        {ws.status}
                      </Badge>
                    </td>
                    <td className="p-4">
                      <span className="font-mono text-sm">{ws.package_manager}</span>
                    </td>
                    <td className="p-4 text-sm text-muted-foreground">
                      {ws.location === 'local' ? (ws.size_formatted || '-') : '-'}
                    </td>
                    <td className="p-4 text-sm text-muted-foreground">
                      {new Date(ws.created_at).toLocaleDateString()}
                    </td>
                    <td className="p-4">
                      <div className="flex justify-end gap-2">
                        {ws.location === 'local' && ws.source !== 'local' && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={(e) => {
                              e.stopPropagation();
                              handleEdit(ws.id, ws.name);
                            }}
                            disabled={loadingEdit || (ws.status !== 'ready' && ws.status !== 'failed')}
                            title={
                              ws.status === 'pending' || ws.status === 'creating' || ws.status === 'deleting'
                                ? 'Cannot edit while workspace is being processed'
                                : 'Edit pixi.toml'
                            }
                          >
                            <Edit className="h-4 w-4" />
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={(e) => {
                            e.stopPropagation();
                            setConfirmDelete({ id: ws.id, name: ws.name, location: ws.location });
                          }}
                          disabled={isDeletePending}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {displayedWorkspaces.length === 0 && !showCreate && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">No workspaces yet. Create your first one!</p>
        </div>
      )}

      <ConfirmDialog
        open={!!confirmDelete}
        onOpenChange={(open) => !open && setConfirmDelete(null)}
        onConfirm={handleDelete}
        title="Delete Workspace"
        description={`Are you sure you want to delete "${confirmDelete?.name}"${confirmDelete?.location === 'remote' ? ' from the remote server' : ''}? This action cannot be undone. All data associated with this workspace will be permanently removed.`}
        confirmText="Delete"
        cancelText="Cancel"
        variant="destructive"
      />
    </div>
  );
};
