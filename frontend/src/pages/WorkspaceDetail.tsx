import { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useWorkspace } from '@/hooks/useWorkspaces';
import { usePackages, useInstallPackages, useRemovePackage } from '@/hooks/usePackages';
import { useCollaborators } from '@/hooks/useAdmin';
import { usePublications } from '@/hooks/useRegistries';
import { workspacesApi } from '@/api/workspaces';
import { useAuthStore } from '@/store/authStore';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { ShareButton } from '@/components/sharing/ShareButton';
import { PublishButton } from '@/components/publishing/PublishButton';
import { RoleBadge } from '@/components/sharing/RoleBadge';
import { VersionHistory } from '@/components/versions/VersionHistory';
import { ArrowLeft, Loader2, Package, Plus, Trash2, Copy, Check, ExternalLink, Save, HardDrive, Pencil } from 'lucide-react';

const statusColors: Record<string, string> = {
  pending: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
  creating: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  ready: 'bg-green-500/10 text-green-500 border-green-500/20',
  failed: 'bg-red-500/10 text-red-500 border-red-500/20',
  deleting: 'bg-orange-500/10 text-orange-500 border-orange-500/20',
};

export const WorkspaceDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const wsId = id || '';

  const { data: workspace, isLoading: wsLoading } = useWorkspace(wsId);
  const { data: packages, isLoading: packagesLoading } = usePackages(wsId);
  const { data: collaborators } = useCollaborators(wsId);
  const { data: publications, isLoading: publicationsLoading } = usePublications(wsId);
  const installMutation = useInstallPackages(wsId);
  const removeMutation = useRemovePackage(wsId);
  const currentUser = useAuthStore((state) => state.user);

  const [activeTab, setActiveTab] = useState('overview');
  const [showInstall, setShowInstall] = useState(false);
  const [packageInput, setPackageInput] = useState('');
  const [confirmRemovePackage, setConfirmRemovePackage] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [pixiToml, setPixiToml] = useState<string>('');
  const [editedToml, setEditedToml] = useState<string>('');
  const [isEditingToml, setIsEditingToml] = useState(false);
  const [savingToml, setSavingToml] = useState(false);
  const [loadingToml, setLoadingToml] = useState(false);
  const [copiedToml, setCopiedToml] = useState(false);

  const isLocalWs = workspace?.source === 'local';

  const isOwner = workspace?.owner_id === currentUser?.id;

  // Load pixi.toml when switching to that tab
  useEffect(() => {
    if (activeTab === 'toml' && !pixiToml && workspace?.status === 'ready') {
      loadPixiToml();
    }
  }, [activeTab, workspace?.status]);

  const loadPixiToml = async () => {
    setLoadingToml(true);
    try {
      const { content } = await workspacesApi.getPixiToml(wsId);
      setPixiToml(content);
    } catch {
      setError('Failed to load pixi.toml');
    } finally {
      setLoadingToml(false);
    }
  };

  const handleCopyToml = async () => {
    await navigator.clipboard.writeText(pixiToml);
    setCopiedToml(true);
    setTimeout(() => setCopiedToml(false), 2000);
  };

  const handleInstall = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!packageInput.trim()) return;

    setError('');
    const packageNames = packageInput.split(',').map(p => p.trim()).filter(Boolean);

    try {
      await installMutation.mutateAsync({ packages: packageNames });
      setPackageInput('');
      setShowInstall(false);
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to install package. Please try again.';
      setError(errorMessage);
    }
  };

  const handleRemove = async () => {
    if (!confirmRemovePackage) return;

    setError('');
    try {
      await removeMutation.mutateAsync(confirmRemovePackage);
      setConfirmRemovePackage(null);
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      const errorMessage = error?.response?.data?.error || 'Failed to remove package. Please try again.';
      setError(errorMessage);
    }
  };

  if (wsLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!workspace) {
    return <div>Workspace not found</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => navigate('/workspaces')}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <h1 className="text-3xl font-bold">{workspace.name}</h1>
          <p className="text-muted-foreground">Workspace details and packages</p>
        </div>
        <div className="flex items-center gap-2">
          {isLocalWs && (
            <Badge variant="outline" className="bg-cyan-500/10 text-cyan-500 border-cyan-500/20 gap-1">
              <HardDrive className="h-3 w-3" />
              Local
            </Badge>
          )}
          <Badge className={statusColors[workspace.status]}>
            {workspace.status}
          </Badge>
          <PublishButton environmentId={wsId} environmentName={workspace.name} environmentStatus={workspace.status} />
          {!isLocalWs && isOwner && <ShareButton environmentId={wsId} />}
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={(tab) => {
        setActiveTab(tab);
        setError('');
      }}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="packages">Packages</TabsTrigger>
          <TabsTrigger value="toml">pixi.toml</TabsTrigger>
          <TabsTrigger value="versions">Version History</TabsTrigger>
          <TabsTrigger value="publications">
            Publications ({publications?.length || 0})
          </TabsTrigger>
          {!isLocalWs && (
            <TabsTrigger value="collaborators">
              Collaborators ({collaborators?.length || 0})
            </TabsTrigger>
          )}
        </TabsList>

        <TabsContent value="overview">
          <Card>
            <CardHeader>
              <CardTitle>Workspace Info</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-3">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Name:</span>
                <span className="font-medium">{workspace.name}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Owner:</span>
                <span className="font-medium">
                  {workspace.owner?.username || (isOwner ? 'You' : 'Unknown')}
                  {isOwner && <span className="ml-2 text-xs text-muted-foreground">(you)</span>}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Status:</span>
                <Badge className={statusColors[workspace.status]}>
                  {workspace.status}
                </Badge>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Package Manager:</span>
                <span className="font-medium font-mono text-sm">{workspace.package_manager}</span>
              </div>
              {isLocalWs && workspace.path && (
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Path:</span>
                  <span className="font-medium font-mono text-sm truncate max-w-md" title={workspace.path}>{workspace.path}</span>
                </div>
              )}
              {isLocalWs && workspace.origin_name && (
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Origin:</span>
                  <span className="font-medium">
                    {workspace.origin_name}
                    {workspace.origin_tag && `:${workspace.origin_tag}`}
                    {workspace.origin_action && ` (${workspace.origin_action})`}
                  </span>
                </div>
              )}
              <div className="flex justify-between">
                <span className="text-muted-foreground">Size:</span>
                <span className="font-medium">{workspace.size_formatted || 'Calculating...'}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Packages:</span>
                <span className="font-medium">{packages?.length || 0} installed</span>
              </div>
              {!isLocalWs && (
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Collaborators:</span>
                  <span className="font-medium">{collaborators?.length || 0}</span>
                </div>
              )}
              <div className="flex justify-between">
                <span className="text-muted-foreground">Created:</span>
                <span>{new Date(workspace.created_at).toLocaleString()}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Last Updated:</span>
                <span>{new Date(workspace.updated_at).toLocaleString()}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">ID:</span>
                <span className="font-mono text-xs text-muted-foreground">{workspace.id}</span>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="packages">
          <div className="space-y-4">
            <div className="flex justify-between items-center">
              <h2 className="text-2xl font-bold">Packages</h2>
              <Button
                onClick={() => {
                  setShowInstall(!showInstall);
                  setError('');
                }}
                disabled={workspace.status !== 'ready'}
              >
                <Plus className="h-4 w-4 mr-2" />
                Install Package
              </Button>
            </div>

        {showInstall && (
          <Card>
            <CardContent className="pt-6">
              <form onSubmit={handleInstall} className="flex gap-2">
                <Input
                  placeholder="Package name (e.g., python=3.11, numpy)"
                  value={packageInput}
                  onChange={(e) => setPackageInput(e.target.value)}
                  autoFocus
                />
                <Button type="submit" disabled={installMutation.isPending}>
                  {installMutation.isPending ? 'Installing...' : 'Install'}
                </Button>
                <Button type="button" variant="outline" onClick={() => {
                  setShowInstall(false);
                  setError('');
                }}>
                  Cancel
                </Button>
              </form>
              <p className="text-sm text-muted-foreground mt-2">
                Separate multiple packages with commas
              </p>
            </CardContent>
          </Card>
        )}

        {error && (
          <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded">
            {error}
          </div>
        )}

        {packagesLoading ? (
          <div className="flex items-center justify-center py-12">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <Card>
            <div className="rounded-md border">
              <table className="w-full">
                <thead className="border-b bg-muted/50">
                  <tr>
                    <th className="text-left p-4 font-medium">Package</th>
                    <th className="text-left p-4 font-medium">Installed Version</th>
                    <th className="text-right p-4 font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {packages?.length === 0 ? (
                    <tr>
                      <td colSpan={3} className="p-8 text-center text-muted-foreground">
                        No packages installed
                      </td>
                    </tr>
                  ) : (
                    packages?.map((pkg) => (
                      <tr key={pkg.id} className="hover:bg-muted/50">
                        <td className="p-4">
                          <div className="flex items-center gap-2">
                            <Package className="h-4 w-4 text-muted-foreground" />
                            <span className="font-medium">{pkg.name}</span>
                          </div>
                        </td>
                        <td className="p-4 text-muted-foreground font-mono text-sm">
                          {pkg.version || '-'}
                        </td>
                        <td className="p-4 text-right">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setConfirmRemovePackage(pkg.name)}
                            disabled={removeMutation.isPending || workspace.status !== 'ready'}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </Card>
        )}

            {packages?.length === 0 && !showInstall && (
              <div className="text-center py-12">
                <p className="text-muted-foreground">No packages installed yet</p>
              </div>
            )}
          </div>
        </TabsContent>

        <TabsContent value="toml">
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle>pixi.toml Configuration</CardTitle>
                <div className="flex items-center gap-2">
                  {isLocalWs && pixiToml && !isEditingToml && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setEditedToml(pixiToml);
                        setIsEditingToml(true);
                      }}
                      className="gap-2"
                    >
                      <Pencil className="h-4 w-4" />
                      Edit
                    </Button>
                  )}
                  {isEditingToml && (
                    <>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setIsEditingToml(false)}
                      >
                        Cancel
                      </Button>
                      <Button
                        size="sm"
                        onClick={async () => {
                          setSavingToml(true);
                          try {
                            await workspacesApi.savePixiToml(wsId, editedToml);
                            setPixiToml(editedToml);
                            setIsEditingToml(false);
                          } catch {
                            setError('Failed to save pixi.toml');
                          } finally {
                            setSavingToml(false);
                          }
                        }}
                        disabled={savingToml}
                        className="gap-2"
                      >
                        {savingToml ? (
                          <Loader2 className="h-4 w-4 animate-spin" />
                        ) : (
                          <Save className="h-4 w-4" />
                        )}
                        Save
                      </Button>
                    </>
                  )}
                  {pixiToml && !isEditingToml && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={handleCopyToml}
                      className="gap-2"
                    >
                      {copiedToml ? (
                        <>
                          <Check className="h-4 w-4" />
                          Copied
                        </>
                      ) : (
                        <>
                          <Copy className="h-4 w-4" />
                          Copy
                        </>
                      )}
                    </Button>
                  )}
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {workspace.status !== 'ready' ? (
                <div className="text-center py-8 text-muted-foreground">
                  Workspace must be ready to view pixi.toml
                </div>
              ) : loadingToml ? (
                <div className="flex items-center justify-center py-12">
                  <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
              ) : isEditingToml ? (
                <textarea
                  className="w-full h-96 bg-slate-900 text-slate-100 p-4 rounded-md font-mono text-sm resize-y border-0 focus:outline-none focus:ring-2 focus:ring-primary"
                  value={editedToml}
                  onChange={(e) => setEditedToml(e.target.value)}
                />
              ) : pixiToml ? (
                <pre className="bg-slate-900 text-slate-100 p-4 rounded-md overflow-x-auto font-mono text-sm whitespace-pre">
                  {pixiToml}
                </pre>
              ) : (
                <div className="text-center py-8 text-muted-foreground">
                  Failed to load pixi.toml
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="versions">
          <VersionHistory environmentId={wsId} environmentStatus={workspace.status} />
        </TabsContent>

        <TabsContent value="publications">
          <Card>
            <CardHeader>
              <CardTitle>Publications</CardTitle>
            </CardHeader>
            <CardContent>
              {publicationsLoading ? (
                <div className="flex items-center justify-center py-12">
                  <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
              ) : publications && publications.length > 0 ? (
                <div className="space-y-3">
                  {publications.map((pub) => (
                    <div
                      key={pub.id}
                      className="p-4 rounded-lg border"
                    >
                      <div className="flex items-start justify-between">
                        <div className="flex-1 space-y-2">
                          <div className="flex items-center gap-2">
                            <a
                              href={`https://${pub.registry_url}/repository/${pub.repository}?tab=tags`}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="font-medium text-lg hover:underline text-primary flex items-center gap-1"
                            >
                              {pub.repository}:{pub.tag}
                              <ExternalLink className="h-4 w-4" />
                            </a>
                          </div>
                          <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-sm">
                            <div>
                              <span className="text-muted-foreground">Registry:</span>
                              <span className="ml-2 font-medium">{pub.registry_name}</span>
                            </div>
                            <div>
                              <span className="text-muted-foreground">URL:</span>
                              <span className="ml-2 font-mono text-xs">{pub.registry_url}</span>
                            </div>
                            <div>
                              <span className="text-muted-foreground">Published by:</span>
                              <span className="ml-2 font-medium">{pub.published_by}</span>
                            </div>
                            <div>
                              <span className="text-muted-foreground">Published:</span>
                              <span className="ml-2">{new Date(pub.published_at).toLocaleString()}</span>
                            </div>
                          </div>
                          <div className="pt-2">
                            <span className="text-muted-foreground text-sm">Digest:</span>
                            <code className="ml-2 text-xs font-mono bg-muted px-2 py-1 rounded">
                              {pub.digest}
                            </code>
                          </div>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground text-center py-8">
                  No publications yet. Click the "Publish" button to publish this workspace to an OCI registry.
                </p>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {!isLocalWs && (
          <TabsContent value="collaborators">
            <Card>
              <CardHeader>
                <CardTitle>Collaborators</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-2">
                  {collaborators?.map((collab) => (
                    <div
                      key={collab.user_id}
                      className="flex justify-between items-center p-3 rounded-lg border"
                    >
                      <div className="flex-1">
                        <div className="font-medium">{collab.username}</div>
                        <div className="text-sm text-muted-foreground">{collab.email}</div>
                      </div>
                      <RoleBadge role={collab.role} />
                    </div>
                  ))}
                </div>
                {(!collaborators || collaborators.length === 0) && (
                  <p className="text-sm text-muted-foreground text-center py-8">
                    No collaborators yet
                  </p>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        )}
      </Tabs>

      <ConfirmDialog
        open={!!confirmRemovePackage}
        onOpenChange={(open) => !open && setConfirmRemovePackage(null)}
        onConfirm={handleRemove}
        title="Remove Package"
        description={`Are you sure you want to remove the package "${confirmRemovePackage}"? This will uninstall it from the workspace.`}
        confirmText="Remove"
        cancelText="Cancel"
        variant="destructive"
      />
    </div>
  );
};
