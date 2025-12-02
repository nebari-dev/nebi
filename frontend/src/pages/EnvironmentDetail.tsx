import { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useEnvironment } from '@/hooks/useEnvironments';
import { usePackages, useInstallPackages, useRemovePackage } from '@/hooks/usePackages';
import { useCollaborators } from '@/hooks/useAdmin';
import { useAuthStore } from '@/store/authStore';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { ShareButton } from '@/components/sharing/ShareButton';
import { RoleBadge } from '@/components/sharing/RoleBadge';
import { VersionHistory } from '@/components/versions/VersionHistory';
import { ArrowLeft, Loader2, Package, Plus, Trash2 } from 'lucide-react';

const statusColors = {
  pending: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
  creating: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  ready: 'bg-green-500/10 text-green-500 border-green-500/20',
  failed: 'bg-red-500/10 text-red-500 border-red-500/20',
  deleting: 'bg-orange-500/10 text-orange-500 border-orange-500/20',
};

export const EnvironmentDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const envId = id || '';

  const { data: environment, isLoading: envLoading } = useEnvironment(envId);
  const { data: packages, isLoading: packagesLoading } = usePackages(envId);
  const { data: collaborators } = useCollaborators(envId);
  const installMutation = useInstallPackages(envId);
  const removeMutation = useRemovePackage(envId);
  const currentUser = useAuthStore((state) => state.user);

  const [activeTab, setActiveTab] = useState('packages');
  const [showInstall, setShowInstall] = useState(false);
  const [packageInput, setPackageInput] = useState('');
  const [confirmRemovePackage, setConfirmRemovePackage] = useState<string | null>(null);
  const [error, setError] = useState('');

  const isOwner = environment?.owner_id === currentUser?.id;

  const handleInstall = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!packageInput.trim()) return;

    setError('');
    const packageNames = packageInput.split(',').map(p => p.trim()).filter(Boolean);

    try {
      await installMutation.mutateAsync({ packages: packageNames });
      setPackageInput('');
      setShowInstall(false);
    } catch (err: any) {
      const errorMessage = err?.response?.data?.error || 'Failed to install package. Please try again.';
      setError(errorMessage);
    }
  };

  const handleRemove = async () => {
    if (!confirmRemovePackage) return;

    setError('');
    try {
      await removeMutation.mutateAsync(confirmRemovePackage);
      setConfirmRemovePackage(null);
    } catch (err: any) {
      const errorMessage = err?.response?.data?.error || 'Failed to remove package. Please try again.';
      setError(errorMessage);
    }
  };

  if (envLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!environment) {
    return <div>Environment not found</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => navigate('/environments')}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <h1 className="text-3xl font-bold">{environment.name}</h1>
          <p className="text-muted-foreground">Environment details and packages</p>
        </div>
        <div className="flex items-center gap-2">
          <Badge className={statusColors[environment.status]}>
            {environment.status}
          </Badge>
          {isOwner && <ShareButton environmentId={envId} />}
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={(tab) => {
        setActiveTab(tab);
        setError('');
      }}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="packages">Packages</TabsTrigger>
          <TabsTrigger value="versions">Version History</TabsTrigger>
          <TabsTrigger value="collaborators">
            Collaborators ({collaborators?.length || 0})
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview">
          <Card>
            <CardHeader>
              <CardTitle>Environment Info</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-3">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Name:</span>
                <span className="font-medium">{environment.name}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Owner:</span>
                <span className="font-medium">
                  {environment.owner?.username || (isOwner ? 'You' : 'Unknown')}
                  {isOwner && <span className="ml-2 text-xs text-muted-foreground">(you)</span>}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Status:</span>
                <Badge className={statusColors[environment.status]}>
                  {environment.status}
                </Badge>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Package Manager:</span>
                <span className="font-medium font-mono text-sm">{environment.package_manager}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Size:</span>
                <span className="font-medium">{environment.size_formatted || 'Calculating...'}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Packages:</span>
                <span className="font-medium">{packages?.length || 0} installed</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Collaborators:</span>
                <span className="font-medium">{collaborators?.length || 0}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Created:</span>
                <span>{new Date(environment.created_at).toLocaleString()}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Last Updated:</span>
                <span>{new Date(environment.updated_at).toLocaleString()}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">ID:</span>
                <span className="font-mono text-xs text-muted-foreground">{environment.id}</span>
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
                disabled={environment.status !== 'ready'}
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
                    <th className="text-left p-4 font-medium">Version</th>
                    <th className="text-left p-4 font-medium">Installed</th>
                    <th className="text-right p-4 font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {packages?.length === 0 ? (
                    <tr>
                      <td colSpan={4} className="p-8 text-center text-muted-foreground">
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
                        <td className="p-4 text-muted-foreground">
                          {pkg.version || '-'}
                        </td>
                        <td className="p-4 text-muted-foreground">
                          {pkg.installed_at && new Date(pkg.installed_at).getFullYear() > 1900
                            ? new Date(pkg.installed_at).toLocaleString()
                            : '-'}
                        </td>
                        <td className="p-4 text-right">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setConfirmRemovePackage(pkg.name)}
                            disabled={removeMutation.isPending || environment.status !== 'ready'}
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

        <TabsContent value="versions">
          <VersionHistory environmentId={envId} environmentStatus={environment.status} />
        </TabsContent>

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
      </Tabs>

      <ConfirmDialog
        open={!!confirmRemovePackage}
        onOpenChange={(open) => !open && setConfirmRemovePackage(null)}
        onConfirm={handleRemove}
        title="Remove Package"
        description={`Are you sure you want to remove the package "${confirmRemovePackage}"? This will uninstall it from the environment.`}
        confirmText="Remove"
        cancelText="Cancel"
        variant="destructive"
      />
    </div>
  );
};
