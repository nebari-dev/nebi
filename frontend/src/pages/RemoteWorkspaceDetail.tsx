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
  User,
} from 'lucide-react';
import { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { remoteApi } from '@/api/remote';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { CodeBlock, CodeBlockBody } from '@/components/ui/code-block';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Tabs, TabsList, TabsPanel, TabsTab } from '@/components/ui/tabs';
import { UserBadge } from '@/components/ui/user-badge';
import {
  capitalize,
  getWorkspaceStatusColor,
  getWorkspaceVersionLabel,
} from '@/lib/utils';
import type { RemoteWorkspaceTag, RemoteWorkspaceVersion } from '@/types';

export const RemoteWorkspaceDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const wsId = id || '';

  const [activeTab, setActiveTab] = useState('overview');
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
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    className="text-muted-foreground hover:text-muted-foreground-strong"
                    onClick={async () => {
                      await navigator.clipboard.writeText(workspace.id);
                      setCopiedId(true);
                      setTimeout(() => setCopiedId(false), 2000);
                    }}
                    aria-label="Copy workspace ID"
                    title="Copy ID"
                  >
                    {copiedId ? (
                      <Check className="h-3 w-3" />
                    ) : (
                      <Copy className="h-3 w-3" />
                    )}
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </TabsPanel>

        <TabsPanel value="toml">
          <Card>
            <CardHeader>
              <CardTitle>pixi.toml Configuration</CardTitle>
            </CardHeader>
            <CardContent>
              {tomlLoading ? (
                <div className="flex items-center justify-center py-12">
                  <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
              ) : pixiTomlData?.content ? (
                <CodeBlock code={pixiTomlData.content} className="w-full">
                  <CodeBlockBody aria-label="pixi.toml contents" />
                </CodeBlock>
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
                <Table aria-label="Version history">
                  <TableHeader>
                    <TableRow>
                      <TableHead>Workspace Version</TableHead>
                      <TableHead>Snapshot</TableHead>
                      <TableHead>Description</TableHead>
                      <TableHead>Created</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {versions.map((v: RemoteWorkspaceVersion) => (
                      <TableRow key={v.id || v.version_number}>
                        <TableCell>
                          <Badge variant="outline">
                            {getWorkspaceVersionLabel(v)}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          #{v.version_number}
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {v.description || '-'}
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {v.created_at
                            ? new Date(v.created_at).toLocaleString()
                            : '-'}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
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
                <Table aria-label="Tags">
                  <TableHeader>
                    <TableRow>
                      <TableHead>Tag</TableHead>
                      <TableHead>Snapshot</TableHead>
                      <TableHead>Created</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {tags.map((t: RemoteWorkspaceTag) => (
                      <TableRow key={t.tag}>
                        <TableCell>
                          <Badge variant="outline">{t.tag}</Badge>
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          #{t.version_number}
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {t.created_at
                            ? new Date(t.created_at).toLocaleString()
                            : '-'}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
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
