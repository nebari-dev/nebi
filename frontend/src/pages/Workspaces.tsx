import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useWorkspaces, useCreateWorkspace, useDeleteWorkspace } from '@/hooks/useWorkspaces';
import { useRemoteWorkspaces } from '@/hooks/useRemote';
import { useServerStatus } from '@/hooks/useRemote';
import { workspacesApi } from '@/api/workspaces';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Loader2, Plus, Trash2, X, Edit, Users, FileCode, Cloud, Monitor } from 'lucide-react';

interface Package {
  name: string;
  version: string;
}

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

const buildPixiToml = (packages: Package[], wsName: string): string => {
  const dependenciesLines = packages
    .filter(pkg => pkg.name.trim())
    .map(pkg => {
      if (pkg.version.trim()) {
        return `${pkg.name} = "${pkg.version}"`;
      }
      // If no version specified, use "*" (any version)
      return `${pkg.name} = "*"`;
    })
    .join('\n');

  return `[workspace]
name = "${wsName}"
channels = ["conda-forge"]
platforms = ["osx-arm64", "linux-64", "win-64"]

[dependencies]
${dependenciesLines || 'python = ">=3.11"'}
`;
};

interface DisplayWorkspace {
  id: string;
  name: string;
  status: string;
  package_manager: string;
  size_formatted?: string;
  created_at: string;
  owner_id?: string;
  owner?: { id?: string; username: string };
  source: 'local' | 'remote';
}

export const Workspaces = () => {
  const navigate = useNavigate();
  const { data: workspaces, isLoading } = useWorkspaces();
  const createMutation = useCreateWorkspace();
  const deleteMutation = useDeleteWorkspace();
  const currentUser = useAuthStore((state) => state.user);
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const { data: serverStatus } = useServerStatus();
  const { data: remoteWorkspaces } = useRemoteWorkspaces();

  // Merge local and remote workspaces for display
  const showRemote = isLocalMode && !!serverStatus?.connected;
  const allWorkspaces: DisplayWorkspace[] = [
    ...(workspaces?.map((ws) => ({ ...ws, source: 'local' as const })) || []),
    ...(showRemote && remoteWorkspaces
      ? remoteWorkspaces.map((ws: any) => ({
          id: ws.id,
          name: ws.name,
          status: ws.status || 'unknown',
          package_manager: ws.package_manager || 'pixi',
          size_formatted: ws.size_formatted,
          created_at: ws.created_at,
          owner_id: ws.owner_id,
          owner: ws.owner,
          source: 'remote' as const,
        }))
      : []),
  ];

  const [showCreate, setShowCreate] = useState(false);
  const [newWsName, setNewWsName] = useState('');
  const [pixiToml, setPixiToml] = useState(DEFAULT_PIXI_TOML);
  const [createMode, setCreateMode] = useState<'ui' | 'toml'>('ui');
  const [packages, setPackages] = useState<Package[]>([{ name: 'python', version: '>=3.11' }]);
  const [newPackageName, setNewPackageName] = useState('');
  const [newPackageVersion, setNewPackageVersion] = useState('');

  const [showEdit, setShowEdit] = useState(false);
  const [editWsId, setEditWsId] = useState<string | null>(null);
  const [editWsName, setEditWsName] = useState('');
  const [editPixiToml, setEditPixiToml] = useState('');
  const [loadingEdit, setLoadingEdit] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<{ id: string; name: string } | null>(null);
  const [error, setError] = useState('');

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newWsName.trim()) return;

    setError('');
    try {
      // Build pixi.toml based on mode
      const tomlContent = createMode === 'ui'
        ? buildPixiToml(packages, newWsName)
        : pixiToml;

      // Create workspace with custom pixi.toml content
      await createMutation.mutateAsync({
        name: newWsName,
        package_manager: 'pixi',
        pixi_toml: tomlContent
      });

      // Reset form
      setNewWsName('');
      setPixiToml(DEFAULT_PIXI_TOML);
      setPackages([{ name: 'python', version: '>=3.11' }]);
      setShowCreate(false);

      // Redirect to jobs page to see the creation progress
      navigate('/jobs');
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to create workspace. Please try again.';
      setError(errorMessage);
    }
  };

  const handleAddPackage = () => {
    if (!newPackageName.trim()) return;
    setPackages([...packages, { name: newPackageName, version: newPackageVersion }]);
    setNewPackageName('');
    setNewPackageVersion('');
  };

  const handleRemovePackageFromList = (index: number) => {
    setPackages(packages.filter((_, i) => i !== index));
  };

  const handleModeSwitch = (mode: 'ui' | 'toml') => {
    // When switching to TOML mode, populate with current packages
    if (mode === 'toml' && createMode === 'ui') {
      setPixiToml(buildPixiToml(packages, newWsName || 'my-project'));
    }
    setCreateMode(mode);
  };

  const handleDelete = async () => {
    if (!confirmDelete) return;

    setError('');
    try {
      await deleteMutation.mutateAsync(confirmDelete.id);
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
      // Delete the old workspace and create a new one with updated content
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

      // Redirect to jobs page to see the progress
      navigate('/jobs');
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to update workspace. Please try again.';
      setError(errorMessage);
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
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold">Workspaces</h1>
          <p className="text-muted-foreground">Manage your development workspaces</p>
        </div>
        <Button onClick={() => {
          setShowCreate(!showCreate);
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

              {/* Mode Toggle */}
              <div className="flex gap-2 p-1 bg-muted rounded-lg w-fit">
                <Button
                  type="button"
                  variant={createMode === 'ui' ? 'default' : 'ghost'}
                  size="sm"
                  onClick={() => handleModeSwitch('ui')}
                  className="gap-2"
                >
                  <Plus className="h-4 w-4" />
                  UI Mode
                </Button>
                <Button
                  type="button"
                  variant={createMode === 'toml' ? 'default' : 'ghost'}
                  size="sm"
                  onClick={() => handleModeSwitch('toml')}
                  className="gap-2"
                >
                  <FileCode className="h-4 w-4" />
                  TOML Mode
                </Button>
              </div>

              {createMode === 'ui' ? (
                <div className="space-y-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Packages</label>
                    <div className="border rounded-lg overflow-hidden">
                      <table className="w-full">
                        <thead className="bg-muted/50 border-b">
                          <tr>
                            <th className="text-left p-3 text-sm font-medium">Name</th>
                            <th className="text-left p-3 text-sm font-medium">Version Constraint</th>
                            <th className="w-16"></th>
                          </tr>
                        </thead>
                        <tbody className="divide-y">
                          {packages.map((pkg, index) => (
                            <tr key={index} className="hover:bg-muted/30">
                              <td className="p-3">
                                <span className="font-mono text-sm">{pkg.name}</span>
                              </td>
                              <td className="p-3">
                                <span className="font-mono text-sm text-muted-foreground">
                                  {pkg.version || '-'}
                                </span>
                              </td>
                              <td className="p-3">
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => handleRemovePackageFromList(index)}
                                >
                                  <Trash2 className="h-4 w-4" />
                                </Button>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>

                  {/* Add Package Form */}
                  <div className="flex gap-2">
                    <Input
                      placeholder="Package name (e.g., numpy)"
                      value={newPackageName}
                      onChange={(e) => setNewPackageName(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          handleAddPackage();
                        }
                      }}
                    />
                    <Input
                      placeholder="Version (e.g., >=1.24.0)"
                      value={newPackageVersion}
                      onChange={(e) => setNewPackageVersion(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          handleAddPackage();
                        }
                      }}
                      className="w-64"
                    />
                    <Button
                      type="button"
                      onClick={handleAddPackage}
                      disabled={!newPackageName.trim()}
                    >
                      <Plus className="h-4 w-4 mr-2" />
                      Add Package
                    </Button>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Add packages with optional version constraints (e.g., {'>'}=1.24.0, ~=2.0.0, 3.11.*)
                  </p>
                </div>
              ) : (
                <div className="space-y-2">
                  <label className="text-sm font-medium">pixi.toml Configuration</label>
                  <Textarea
                    placeholder="Enter your pixi.toml content"
                    value={pixiToml}
                    onChange={(e) => setPixiToml(e.target.value)}
                    rows={12}
                    required
                    className="font-mono text-sm"
                  />
                  <p className="text-xs text-muted-foreground">
                    Define your project dependencies and configuration in TOML format
                  </p>
                </div>
              )}

              <div className="flex gap-2 justify-end">
                <Button type="button" variant="outline" onClick={() => setShowCreate(false)}>
                  Cancel
                </Button>
                <Button type="submit" disabled={createMutation.isPending}>
                  {createMutation.isPending ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Creating...
                    </>
                  ) : (
                    'Create Workspace'
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

              <div className="space-y-2">
                <label className="text-sm font-medium">pixi.toml Configuration</label>
                <Textarea
                  placeholder="Enter your pixi.toml content"
                  value={editPixiToml}
                  onChange={(e) => setEditPixiToml(e.target.value)}
                  rows={12}
                  required
                />
                <p className="text-xs text-muted-foreground">
                  Editing will delete the old workspace and create a new one with updated configuration
                </p>
              </div>

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
                {allWorkspaces.map((ws) => (
                  <tr
                    key={`${ws.source}-${ws.id}`}
                    className="border-b last:border-0 hover:bg-muted/50 cursor-pointer transition-colors"
                    onClick={() =>
                      ws.source === 'remote'
                        ? navigate(`/remote/workspaces/${ws.id}`)
                        : navigate(`/workspaces/${ws.id}`)
                    }
                  >
                    <td className="p-4 font-medium">
                      <div className="flex items-center gap-2">
                        {ws.name}
                        {showRemote && (
                          ws.source === 'local' ? (
                            <Badge variant="outline" className="bg-slate-500/10 text-slate-500 border-slate-500/20">
                              <Monitor className="h-3 w-3 mr-1" />
                              Local
                            </Badge>
                          ) : (
                            <Badge variant="outline" className="bg-purple-500/10 text-purple-500 border-purple-500/20">
                              <Cloud className="h-3 w-3 mr-1" />
                              Remote
                            </Badge>
                          )
                        )}
                        {ws.source === 'local' && ws.owner_id !== currentUser?.id && ws.owner && (
                          <Badge variant="outline" className="bg-blue-500/10 text-blue-500 border-blue-500/20">
                            <Users className="h-3 w-3 mr-1" />
                            {ws.owner.username}
                          </Badge>
                        )}
                      </div>
                    </td>
                    <td className="p-4">
                      <Badge className={statusColors[ws.status] || 'bg-gray-500/10 text-gray-500 border-gray-500/20'}>
                        {ws.status}
                      </Badge>
                    </td>
                    <td className="p-4">
                      <span className="font-mono text-sm">{ws.package_manager}</span>
                    </td>
                    <td className="p-4 text-sm text-muted-foreground">
                      {ws.size_formatted || '-'}
                    </td>
                    <td className="p-4 text-sm text-muted-foreground">
                      {ws.created_at ? new Date(ws.created_at).toLocaleDateString() : '-'}
                    </td>
                    <td className="p-4">
                      {ws.source === 'local' ? (
                        <div className="flex justify-end gap-2">
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
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={(e) => {
                              e.stopPropagation();
                              setConfirmDelete({ id: ws.id, name: ws.name });
                            }}
                            disabled={deleteMutation.isPending}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      ) : (
                        <div className="flex justify-end">
                          <span className="text-xs text-muted-foreground">read-only</span>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {allWorkspaces.length === 0 && !showCreate && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">No workspaces yet. Create your first one!</p>
        </div>
      )}

      <ConfirmDialog
        open={!!confirmDelete}
        onOpenChange={(open) => !open && setConfirmDelete(null)}
        onConfirm={handleDelete}
        title="Delete Workspace"
        description={`Are you sure you want to delete "${confirmDelete?.name}"? This action cannot be undone. All data associated with this workspace will be permanently removed.`}
        confirmText="Delete"
        cancelText="Cancel"
        variant="destructive"
      />
    </div>
  );
};
