import { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { remoteApi } from '@/api/remote';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { ArrowLeft, Loader2, Cloud, Copy, Check } from 'lucide-react';

const statusColors: Record<string, string> = {
  pending: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
  creating: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  ready: 'bg-green-500/10 text-green-500 border-green-500/20',
  failed: 'bg-red-500/10 text-red-500 border-red-500/20',
  deleting: 'bg-orange-500/10 text-orange-500 border-orange-500/20',
};

export const RemoteWorkspaceDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const wsId = id || '';

  const [activeTab, setActiveTab] = useState('overview');
  const [copiedToml, setCopiedToml] = useState(false);

  const { data: workspace, isLoading: wsLoading } = useQuery({
    queryKey: ['remote', 'workspaces', wsId],
    queryFn: () => remoteApi.getWorkspace(wsId),
    enabled: !!wsId,
  });

  const { data: versions, isLoading: versionsLoading } = useQuery({
    queryKey: ['remote', 'workspaces', wsId, 'versions'],
    queryFn: () => remoteApi.listVersions(wsId),
    enabled: !!wsId && activeTab === 'versions',
  });

  const { data: tags, isLoading: tagsLoading } = useQuery({
    queryKey: ['remote', 'workspaces', wsId, 'tags'],
    queryFn: () => remoteApi.listTags(wsId),
    enabled: !!wsId && activeTab === 'tags',
  });

  const { data: pixiTomlData, isLoading: tomlLoading } = useQuery({
    queryKey: ['remote', 'workspaces', wsId, 'pixi-toml'],
    queryFn: () => remoteApi.getPixiToml(wsId),
    enabled: !!wsId && activeTab === 'toml',
  });

  const handleCopyToml = async () => {
    if (pixiTomlData?.content) {
      await navigator.clipboard.writeText(pixiTomlData.content);
      setCopiedToml(true);
      setTimeout(() => setCopiedToml(false), 2000);
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
    return <div>Remote workspace not found</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => navigate('/workspaces')}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <h1 className="text-3xl font-bold">{workspace.name}</h1>
          <p className="text-muted-foreground">Remote workspace details (read-only)</p>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="outline" className="bg-purple-500/10 text-purple-500 border-purple-500/20">
            <Cloud className="h-3 w-3 mr-1" />
            Remote
          </Badge>
          <Badge className={statusColors[workspace.status] || 'bg-gray-500/10 text-gray-500 border-gray-500/20'}>
            {workspace.status}
          </Badge>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="toml">pixi.toml</TabsTrigger>
          <TabsTrigger value="versions">Version History</TabsTrigger>
          <TabsTrigger value="tags">Tags</TabsTrigger>
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
                <span className="text-muted-foreground">Status:</span>
                <Badge className={statusColors[workspace.status] || 'bg-gray-500/10 text-gray-500 border-gray-500/20'}>
                  {workspace.status}
                </Badge>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Package Manager:</span>
                <span className="font-medium font-mono text-sm">{workspace.package_manager || 'pixi'}</span>
              </div>
              {workspace.size_bytes > 0 && (
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Size:</span>
                  <span className="font-medium">{(workspace.size_bytes / 1024 / 1024).toFixed(1)} MB</span>
                </div>
              )}
              {workspace.owner?.username && (
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Owner:</span>
                  <span className="font-medium">{workspace.owner.username}</span>
                </div>
              )}
              {workspace.created_at && (
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Created:</span>
                  <span>{new Date(workspace.created_at).toLocaleString()}</span>
                </div>
              )}
              {workspace.updated_at && (
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Last Updated:</span>
                  <span>{new Date(workspace.updated_at).toLocaleString()}</span>
                </div>
              )}
              <div className="flex justify-between">
                <span className="text-muted-foreground">ID:</span>
                <span className="font-mono text-xs text-muted-foreground">{workspace.id}</span>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="toml">
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle>pixi.toml Configuration</CardTitle>
                {pixiTomlData?.content && (
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
            </CardHeader>
            <CardContent>
              {tomlLoading ? (
                <div className="flex items-center justify-center py-12">
                  <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
              ) : pixiTomlData?.content ? (
                <pre className="bg-slate-900 text-slate-100 p-4 rounded-md overflow-x-auto font-mono text-sm whitespace-pre">
                  {pixiTomlData.content}
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
          <Card>
            <CardHeader>
              <CardTitle>Version History</CardTitle>
            </CardHeader>
            <CardContent>
              {versionsLoading ? (
                <div className="flex items-center justify-center py-12">
                  <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
              ) : versions && versions.length > 0 ? (
                <div className="rounded-md border">
                  <table className="w-full">
                    <thead className="border-b bg-muted/50">
                      <tr>
                        <th className="text-left p-4 font-medium">Version</th>
                        <th className="text-left p-4 font-medium">Description</th>
                        <th className="text-left p-4 font-medium">Created</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y">
                      {versions.map((v: any) => (
                        <tr key={v.id || v.version_number} className="hover:bg-muted/50">
                          <td className="p-4">
                            <Badge variant="outline">v{v.version_number}</Badge>
                          </td>
                          <td className="p-4 text-sm text-muted-foreground">
                            {v.description || '-'}
                          </td>
                          <td className="p-4 text-sm text-muted-foreground">
                            {v.created_at ? new Date(v.created_at).toLocaleString() : '-'}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground text-center py-8">
                  No versions available
                </p>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="tags">
          <Card>
            <CardHeader>
              <CardTitle>Tags</CardTitle>
            </CardHeader>
            <CardContent>
              {tagsLoading ? (
                <div className="flex items-center justify-center py-12">
                  <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
              ) : tags && tags.length > 0 ? (
                <div className="rounded-md border">
                  <table className="w-full">
                    <thead className="border-b bg-muted/50">
                      <tr>
                        <th className="text-left p-4 font-medium">Tag</th>
                        <th className="text-left p-4 font-medium">Version</th>
                        <th className="text-left p-4 font-medium">Created</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y">
                      {tags.map((t: any) => (
                        <tr key={t.tag} className="hover:bg-muted/50">
                          <td className="p-4">
                            <Badge variant="outline">{t.tag}</Badge>
                          </td>
                          <td className="p-4 text-sm text-muted-foreground">
                            v{t.version_number}
                          </td>
                          <td className="p-4 text-sm text-muted-foreground">
                            {t.created_at ? new Date(t.created_at).toLocaleString() : '-'}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground text-center py-8">
                  No tags available
                </p>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
};
