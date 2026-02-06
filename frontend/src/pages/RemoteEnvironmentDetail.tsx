import { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useRemoteWorkspace, useRemoteVersions, useRemoteTags } from '@/hooks/useRemote';
import { remoteApi } from '@/api/remote';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { ArrowLeft, Loader2, Copy, Check, Cloud } from 'lucide-react';

const statusColors: Record<string, string> = {
  pending: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
  creating: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  ready: 'bg-green-500/10 text-green-500 border-green-500/20',
  failed: 'bg-red-500/10 text-red-500 border-red-500/20',
  deleting: 'bg-orange-500/10 text-orange-500 border-orange-500/20',
};

export const RemoteEnvironmentDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const wsId = id || '';

  const { data: workspace, isLoading: wsLoading } = useRemoteWorkspace(wsId);
  const { data: versions, isLoading: versionsLoading } = useRemoteVersions(wsId);
  const { data: tags } = useRemoteTags(wsId);

  const [activeTab, setActiveTab] = useState('overview');
  const [pixiToml, setPixiToml] = useState<string>('');
  const [loadingToml, setLoadingToml] = useState(false);
  const [copiedToml, setCopiedToml] = useState(false);

  const loadPixiToml = async () => {
    if (pixiToml) return;
    setLoadingToml(true);
    try {
      const { content } = await remoteApi.getPixiToml(wsId);
      setPixiToml(content);
    } catch {
      // ignore
    } finally {
      setLoadingToml(false);
    }
  };

  const handleCopyToml = async () => {
    await navigator.clipboard.writeText(pixiToml);
    setCopiedToml(true);
    setTimeout(() => setCopiedToml(false), 2000);
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
          <div className="flex items-center gap-2">
            <h1 className="text-3xl font-bold">{workspace.name}</h1>
            <Badge className="bg-purple-500/10 text-purple-500 border-purple-500/20 gap-1">
              <Cloud className="h-3 w-3" />
              Remote
            </Badge>
          </div>
          <p className="text-muted-foreground">Read-only view from remote server</p>
        </div>
        <Badge className={statusColors[workspace.status] || statusColors.ready}>
          {workspace.status}
        </Badge>
      </div>

      <Tabs value={activeTab} onValueChange={(tab) => {
        setActiveTab(tab);
        if (tab === 'toml') loadPixiToml();
      }}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="versions">Versions ({versions?.length || 0})</TabsTrigger>
          <TabsTrigger value="tags">Tags ({tags?.length || 0})</TabsTrigger>
          <TabsTrigger value="toml">pixi.toml</TabsTrigger>
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
              {workspace.owner && (
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Owner:</span>
                  <span className="font-medium">{workspace.owner.username}</span>
                </div>
              )}
              <div className="flex justify-between">
                <span className="text-muted-foreground">Status:</span>
                <Badge className={statusColors[workspace.status] || statusColors.ready}>
                  {workspace.status}
                </Badge>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Package Manager:</span>
                <span className="font-medium font-mono text-sm">{workspace.package_manager}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Versions:</span>
                <span className="font-medium">{versions?.length || 0}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Created:</span>
                <span>{new Date(workspace.created_at).toLocaleString()}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">ID:</span>
                <span className="font-mono text-xs text-muted-foreground">{workspace.id}</span>
              </div>
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
                        <th className="text-left p-4 font-medium">Tags</th>
                        <th className="text-left p-4 font-medium">Created</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y">
                      {versions.map((v) => (
                        <tr key={v.id} className="hover:bg-muted/50">
                          <td className="p-4 font-mono font-medium">v{v.version_number}</td>
                          <td className="p-4">
                            <div className="flex gap-1 flex-wrap">
                              {tags
                                ?.filter((t) => t.version_number === v.version_number)
                                .map((t) => (
                                  <Badge key={t.tag} variant="outline" className="text-xs">
                                    {t.tag}
                                  </Badge>
                                ))}
                            </div>
                          </td>
                          <td className="p-4 text-sm text-muted-foreground">
                            {new Date(v.created_at).toLocaleString()}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground text-center py-8">
                  No versions found
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
              {tags && tags.length > 0 ? (
                <div className="rounded-md border">
                  <table className="w-full">
                    <thead className="border-b bg-muted/50">
                      <tr>
                        <th className="text-left p-4 font-medium">Tag</th>
                        <th className="text-left p-4 font-medium">Version</th>
                        <th className="text-left p-4 font-medium">Updated</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y">
                      {tags.map((t) => (
                        <tr key={t.tag} className="hover:bg-muted/50">
                          <td className="p-4">
                            <Badge variant="outline">{t.tag}</Badge>
                          </td>
                          <td className="p-4 font-mono">v{t.version_number}</td>
                          <td className="p-4 text-sm text-muted-foreground">
                            {new Date(t.updated_at).toLocaleString()}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground text-center py-8">
                  No tags found
                </p>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="toml">
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle>pixi.toml Configuration</CardTitle>
                {pixiToml && (
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
              {loadingToml ? (
                <div className="flex items-center justify-center py-12">
                  <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
              ) : pixiToml ? (
                <pre className="bg-slate-900 text-slate-100 p-4 rounded-md overflow-x-auto font-mono text-sm whitespace-pre">
                  {pixiToml}
                </pre>
              ) : (
                <div className="text-center py-8 text-muted-foreground">
                  No pixi.toml available
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
};
