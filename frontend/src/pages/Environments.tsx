import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useEnvironments, useCreateEnvironment, useDeleteEnvironment } from '@/hooks/useEnvironments';
import { environmentsApi } from '@/api/environments';
import { useAuthStore } from '@/store/authStore';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Loader2, Plus, Trash2, X, Edit, Users, FileCode } from 'lucide-react';

interface Package {
  name: string;
  version: string;
}

const statusColors = {
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

const buildPixiToml = (packages: Package[], envName: string): string => {
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

  return `[project]
name = "${envName}"
channels = ["conda-forge"]
platforms = ["osx-arm64", "linux-64", "win-64"]

[dependencies]
${dependenciesLines || 'python = ">=3.11"'}
`;
};

export const Environments = () => {
  const navigate = useNavigate();
  const { data: environments, isLoading } = useEnvironments();
  const createMutation = useCreateEnvironment();
  const deleteMutation = useDeleteEnvironment();
  const currentUser = useAuthStore((state) => state.user);

  const [showCreate, setShowCreate] = useState(false);
  const [newEnvName, setNewEnvName] = useState('');
  const [pixiToml, setPixiToml] = useState(DEFAULT_PIXI_TOML);
  const [createMode, setCreateMode] = useState<'ui' | 'toml'>('ui');
  const [packages, setPackages] = useState<Package[]>([{ name: 'python', version: '>=3.11' }]);
  const [newPackageName, setNewPackageName] = useState('');
  const [newPackageVersion, setNewPackageVersion] = useState('');

  const [showEdit, setShowEdit] = useState(false);
  const [editEnvId, setEditEnvId] = useState<string | null>(null);
  const [editEnvName, setEditEnvName] = useState('');
  const [editPixiToml, setEditPixiToml] = useState('');
  const [loadingEdit, setLoadingEdit] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<{ id: string; name: string } | null>(null);
  const [error, setError] = useState('');

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newEnvName.trim()) return;

    setError('');
    try {
      // Build pixi.toml based on mode
      const tomlContent = createMode === 'ui'
        ? buildPixiToml(packages, newEnvName)
        : pixiToml;

      // Create environment with custom pixi.toml content
      await createMutation.mutateAsync({
        name: newEnvName,
        package_manager: 'pixi',
        pixi_toml: tomlContent
      });

      // Reset form
      setNewEnvName('');
      setPixiToml(DEFAULT_PIXI_TOML);
      setPackages([{ name: 'python', version: '>=3.11' }]);
      setShowCreate(false);

      // Redirect to jobs page to see the creation progress
      navigate('/jobs');
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to create environment. Please try again.';
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
      setPixiToml(buildPixiToml(packages, newEnvName || 'my-project'));
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
      const errorMessage = error?.response?.data?.error || 'Failed to delete environment. Please try again.';
      setError(errorMessage);
      setConfirmDelete(null);
    }
  };

  const handleEdit = async (id: string, name: string) => {
    setLoadingEdit(true);
    setError('');
    try {
      const { content } = await environmentsApi.getPixiToml(id);
      setEditEnvId(id);
      setEditEnvName(name);
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
    if (!editEnvId || !editEnvName.trim()) return;

    setError('');
    try {
      // Delete the old environment and create a new one with updated content
      await deleteMutation.mutateAsync(editEnvId);

      await createMutation.mutateAsync({
        name: editEnvName,
        package_manager: 'pixi',
        pixi_toml: editPixiToml
      });

      setShowEdit(false);
      setEditEnvId(null);
      setEditEnvName('');
      setEditPixiToml('');

      // Redirect to jobs page to see the progress
      navigate('/jobs');
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to update environment. Please try again.';
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
          <h1 className="text-3xl font-bold">Environments</h1>
          <p className="text-muted-foreground">Manage your development environments</p>
        </div>
        <Button onClick={() => {
          setShowCreate(!showCreate);
          setError('');
        }}>
          <Plus className="h-4 w-4 mr-2" />
          New Environment
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
              <CardTitle>Create New Environment</CardTitle>
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
                <label className="text-sm font-medium">Environment Name</label>
                <Input
                  placeholder="e.g., my-data-project"
                  value={newEnvName}
                  onChange={(e) => setNewEnvName(e.target.value)}
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
                    'Create Environment'
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
              <CardTitle>Edit Environment - {editEnvName}</CardTitle>
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
                <label className="text-sm font-medium">Environment Name</label>
                <Input
                  placeholder="e.g., my-data-project"
                  value={editEnvName}
                  onChange={(e) => setEditEnvName(e.target.value)}
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
                  Editing will delete the old environment and create a new one with updated configuration
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
                {environments?.map((env) => (
                  <tr
                    key={env.id}
                    className="border-b last:border-0 hover:bg-muted/50 cursor-pointer transition-colors"
                    onClick={() => navigate(`/environments/${env.id}`)}
                  >
                    <td className="p-4 font-medium">
                      <div className="flex items-center gap-2">
                        {env.name}
                        {env.owner_id !== currentUser?.id && env.owner && (
                          <Badge variant="outline" className="bg-blue-500/10 text-blue-500 border-blue-500/20">
                            <Users className="h-3 w-3 mr-1" />
                            {env.owner.username}
                          </Badge>
                        )}
                      </div>
                    </td>
                    <td className="p-4">
                      <Badge className={statusColors[env.status]}>
                        {env.status}
                      </Badge>
                    </td>
                    <td className="p-4">
                      <span className="font-mono text-sm">{env.package_manager}</span>
                    </td>
                    <td className="p-4 text-sm text-muted-foreground">
                      {env.size_formatted || '-'}
                    </td>
                    <td className="p-4 text-sm text-muted-foreground">
                      {new Date(env.created_at).toLocaleDateString()}
                    </td>
                    <td className="p-4">
                      <div className="flex justify-end gap-2">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={(e) => {
                            e.stopPropagation();
                            handleEdit(env.id, env.name);
                          }}
                          disabled={loadingEdit || (env.status !== 'ready' && env.status !== 'failed')}
                          title={
                            env.status === 'pending' || env.status === 'creating' || env.status === 'deleting'
                              ? 'Cannot edit while environment is being processed'
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
                            setConfirmDelete({ id: env.id, name: env.name });
                          }}
                          disabled={deleteMutation.isPending}
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

      {environments?.length === 0 && !showCreate && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">No environments yet. Create your first one!</p>
        </div>
      )}

      <ConfirmDialog
        open={!!confirmDelete}
        onOpenChange={(open) => !open && setConfirmDelete(null)}
        onConfirm={handleDelete}
        title="Delete Environment"
        description={`Are you sure you want to delete "${confirmDelete?.name}"? This action cannot be undone. All data associated with this environment will be permanently removed.`}
        confirmText="Delete"
        cancelText="Cancel"
        variant="destructive"
      />
    </div>
  );
};
