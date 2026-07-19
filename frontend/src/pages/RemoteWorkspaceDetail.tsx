import { useQuery } from '@tanstack/react-query';
import {
  ArrowLeft,
  Calendar,
  Check,
  CircleQuestionMark,
  Cloud,
  Copy,
  Fingerprint,
  HardDrive,
  History,
  IdCard,
  Loader2,
  Package,
  User,
} from 'lucide-react';
import { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { remoteApi } from '@/api/remote';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Tabs, TabsList, TabsPanel, TabsTab } from '@/components/ui/tabs';
import { UserBadge } from '@/components/ui/user-badge';
import { capitalize, getWorkspaceStatusColor } from '@/lib/utils';
import type { RemoteWorkspaceTag, RemoteWorkspaceVersion } from '@/types';

export const RemoteWorkspaceDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const wsId = id || '';

  const [activeTab, setActiveTab] = useState('overview');
  const [copiedToml, setCopiedToml] = useState(false);
  const [copiedId, setCopiedId] = useState(false);

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
        <Button
          variant="ghost"
          size="icon"
          onClick={() => navigate('/workspaces')}
          aria-label="Back to workspaces"
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <h1 className="text-3xl font-bold">{workspace.name}</h1>
          <p className="text-muted-foreground">
            Remote workspace details (read-only)
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Badge
            variant="outline"
            className="bg-purple-100 text-purple-800 border-purple-300"
          >
            <Cloud className="h-3 w-3 mr-1" />
            Remote
          </Badge>
          <Badge className={getWorkspaceStatusColor(workspace.status)}>
            {capitalize(workspace.status)}
          </Badge>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          <TabsTab value="overview">Overview</TabsTab>
          <TabsTab value="toml">Configuration</TabsTab>
          <TabsTab value="versions">Version History</TabsTab>
          <TabsTab value="tags">Tags</TabsTab>
        </TabsList>

        <TabsPanel value="overview" className="px-1">
          <div className="space-y-4 my-4">
            <h2 className="text-2xl font-bold mb-0">Overview</h2>
            <p className="text-muted-foreground text-sm mt-2">
              View details for this remote workspace
            </p>
          </div>
          <div>
            <div>
              {/* Name */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <IdCard className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">Workspace Name</span>
                </div>
                <span className="text-sm">{workspace.name}</span>
              </div>

              {/* Owner */}
              {workspace.owner?.username && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <User className="h-3 w-3 shrink-0" />
                    <span className="text-sm font-medium">Owner</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <UserBadge username={workspace.owner.username} />
                  </div>
                </div>
              )}

              {/* Status */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <CircleQuestionMark className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">Status</span>
                </div>
                <div>
                  <Badge className={getWorkspaceStatusColor(workspace.status)}>
                    {capitalize(workspace.status)}
                  </Badge>
                </div>
              </div>

              {/* Package Manager */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <Package className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">Package Manager</span>
                </div>
                <code className="text-sm font-mono">
                  {workspace.package_manager || 'pixi'}
                </code>
              </div>

              {/* Size */}
              {workspace.size_bytes > 0 && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <HardDrive className="h-3 w-3 shrink-0" />
                    <span className="text-sm font-medium">Size</span>
                  </div>
                  <span className="text-sm">
                    {(workspace.size_bytes / 1024 / 1024).toFixed(1)} MB
                  </span>
                </div>
              )}

              {/* Created */}
              {workspace.created_at && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <Calendar className="h-3 w-3 shrink-0" />
                    <span className="text-sm font-medium">Created</span>
                  </div>
                  <span className="text-sm">
                    {new Date(workspace.created_at).toLocaleString()}
                  </span>
                </div>
              )}

              {/* Last Updated */}
              {workspace.updated_at && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <History className="h-3 w-3 shrink-0" />
                    <span className="text-sm font-medium">Last Updated</span>
                  </div>
                  <span className="text-sm">
                    {new Date(workspace.updated_at).toLocaleString()}
                  </span>
                </div>
              )}

              {/* ID */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <Fingerprint className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">ID</span>
                </div>
                <div className="flex items-center gap-2">
                  <code className="text-xs font-mono text-muted-foreground">
                    {workspace.id}
                  </code>
                  <button
                    type="button"
                    className="p-1 rounded hover:bg-muted transition-colors text-muted-foreground"
                    onClick={async () => {
                      await navigator.clipboard.writeText(workspace.id);
                      setCopiedId(true);
                      setTimeout(() => setCopiedId(false), 2000);
                    }}
                    title="Copy ID"
                  >
                    {copiedId ? (
                      <Check className="h-3 w-3" />
                    ) : (
                      <Copy className="h-3 w-3" />
                    )}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </TabsPanel>

        <TabsPanel value="toml">
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
        </TabsPanel>

        <TabsPanel value="versions">
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
                        <th className="text-left p-4 font-medium">
                          Description
                        </th>
                        <th className="text-left p-4 font-medium">Created</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y">
                      {versions.map((v: RemoteWorkspaceVersion) => (
                        <tr
                          key={v.id || v.version_number}
                          className="hover:bg-muted/50"
                        >
                          <td className="p-4">
                            <Badge variant="outline">v{v.version_number}</Badge>
                          </td>
                          <td className="p-4 text-sm text-muted-foreground">
                            {v.description || '-'}
                          </td>
                          <td className="p-4 text-sm text-muted-foreground">
                            {v.created_at
                              ? new Date(v.created_at).toLocaleString()
                              : '-'}
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
        </TabsPanel>

        <TabsPanel value="tags">
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
                      {tags.map((t: RemoteWorkspaceTag) => (
                        <tr key={t.tag} className="hover:bg-muted/50">
                          <td className="p-4">
                            <Badge variant="outline">{t.tag}</Badge>
                          </td>
                          <td className="p-4 text-sm text-muted-foreground">
                            v{t.version_number}
                          </td>
                          <td className="p-4 text-sm text-muted-foreground">
                            {t.created_at
                              ? new Date(t.created_at).toLocaleString()
                              : '-'}
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
        </TabsPanel>
      </Tabs>
    </div>
  );
};
