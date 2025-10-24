import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useEnvironments, useCreateEnvironment, useDeleteEnvironment } from '@/hooks/useEnvironments';
import { environmentsApi } from '@/api/environments';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Loader2, Plus, Trash2, X, Edit } from 'lucide-react';

const statusColors = {
  pending: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
  creating: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  ready: 'bg-green-500/10 text-green-500 border-green-500/20',
  failed: 'bg-red-500/10 text-red-500 border-red-500/20',
  deleting: 'bg-orange-500/10 text-orange-500 border-orange-500/20',
};

const DEFAULT_PIXI_TOML = `[project]
name = "my-project"
channels = ["conda-forge"]
platforms = ["osx-arm64", "linux-64"]

[dependencies]
python = ">=3.11"
`;

export const Environments = () => {
  const navigate = useNavigate();
  const { data: environments, isLoading } = useEnvironments();
  const createMutation = useCreateEnvironment();
  const deleteMutation = useDeleteEnvironment();

  const [showCreate, setShowCreate] = useState(false);
  const [newEnvName, setNewEnvName] = useState('');
  const [pixiToml, setPixiToml] = useState(DEFAULT_PIXI_TOML);

  const [showEdit, setShowEdit] = useState(false);
  const [editEnvId, setEditEnvId] = useState<string | null>(null);
  const [editEnvName, setEditEnvName] = useState('');
  const [editPixiToml, setEditPixiToml] = useState('');
  const [loadingEdit, setLoadingEdit] = useState(false);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newEnvName.trim()) return;

    // Create environment with custom pixi.toml content
    await createMutation.mutateAsync({
      name: newEnvName,
      package_manager: 'pixi',
      pixi_toml: pixiToml
    });

    setNewEnvName('');
    setPixiToml(DEFAULT_PIXI_TOML);
    setShowCreate(false);

    // Redirect to jobs page to see the creation progress
    navigate('/jobs');
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to delete this environment?')) return;
    await deleteMutation.mutateAsync(id);
  };

  const handleEdit = async (id: string, name: string) => {
    setLoadingEdit(true);
    try {
      const { content } = await environmentsApi.getPixiToml(id);
      setEditEnvId(id);
      setEditEnvName(name);
      setEditPixiToml(content);
      setShowEdit(true);
    } catch (error) {
      alert('Failed to load pixi.toml content');
    } finally {
      setLoadingEdit(false);
    }
  };

  const handleEditSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editEnvId || !editEnvName.trim()) return;

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
        <Button onClick={() => setShowCreate(!showCreate)}>
          <Plus className="h-4 w-4 mr-2" />
          New Environment
        </Button>
      </div>

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

              <div className="space-y-2">
                <label className="text-sm font-medium">pixi.toml Configuration</label>
                <Textarea
                  placeholder="Enter your pixi.toml content"
                  value={pixiToml}
                  onChange={(e) => setPixiToml(e.target.value)}
                  rows={12}
                  required
                />
                <p className="text-xs text-muted-foreground">
                  Define your project dependencies and configuration in TOML format
                </p>
              </div>

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
                    <td className="p-4 font-medium">{env.name}</td>
                    <td className="p-4">
                      <Badge className={statusColors[env.status]}>
                        {env.status}
                      </Badge>
                    </td>
                    <td className="p-4">
                      <span className="font-mono text-sm">{env.package_manager}</span>
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
                            handleDelete(env.id);
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
    </div>
  );
};
