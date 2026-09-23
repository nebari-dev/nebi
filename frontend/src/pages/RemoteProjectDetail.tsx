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
import { capitalize, getProjectStatusColor } from '@/lib/utils';
import type { RemoteProjectTag, RemoteProjectVersion } from '@/types';

export const RemoteProjectDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const projectId = id || '';

  const [activeTab, setActiveTab] = useState('overview');
  const [copiedId, setCopiedId] = useState(false);

  const { data: project, isLoading: projectLoading } = useQuery({
    queryKey: ['remote', 'projects', projectId],
    queryFn: () => remoteApi.getProject(projectId),
    enabled: !!projectId,
  });

  const { data: versions, isLoading: versionsLoading } = useQuery({
    queryKey: ['remote', 'projects', projectId, 'versions'],
    queryFn: () => remoteApi.listVersions(projectId),
    enabled: !!projectId && activeTab === 'versions',
  });

  const { data: tags, isLoading: tagsLoading } = useQuery({
    queryKey: ['remote', 'projects', projectId, 'tags'],
    queryFn: () => remoteApi.listTags(projectId),
    enabled: !!projectId && activeTab === 'tags',
  });

  const { data: pixiTomlData, isLoading: tomlLoading } = useQuery({
    queryKey: ['remote', 'projects', projectId, 'pixi-toml'],
    queryFn: () => remoteApi.getPixiToml(projectId),
    enabled: !!projectId && activeTab === 'toml',
  });

  if (projectLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!project) {
    return <div>Remote project not found</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="icon"
          onClick={() => navigate('/projects')}
          aria-label="Back to projects"
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <h1 className="text-3xl font-bold">{project.name}</h1>
          <p className="text-muted-foreground">
            Remote project details (read-only)
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
          <Badge className={getProjectStatusColor(project.status)}>
            {capitalize(project.status)}
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
              View details for this remote project
            </p>
          </div>
          <div>
            <div>
              {/* Name */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <IdCard className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">Project Name</span>
                </div>
                <span className="text-sm">{project.name}</span>
              </div>

              {/* Owner */}
              {project.owner?.username && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <User className="h-3 w-3 shrink-0" />
                    <span className="text-sm font-medium">Owner</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <UserBadge username={project.owner.username} />
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
                  <Badge className={getProjectStatusColor(project.status)}>
                    {capitalize(project.status)}
                  </Badge>
                </div>
              </div>

              {/* Size */}
              {project.size_bytes > 0 && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <HardDrive className="h-3 w-3 shrink-0" />
                    <span className="text-sm font-medium">Size</span>
                  </div>
                  <span className="text-sm">
                    {(project.size_bytes / 1024 / 1024).toFixed(1)} MB
                  </span>
                </div>
              )}

              {/* Created */}
              {project.created_at && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <Calendar className="h-3 w-3 shrink-0" />
                    <span className="text-sm font-medium">Created</span>
                  </div>
                  <span className="text-sm">
                    {new Date(project.created_at).toLocaleString()}
                  </span>
                </div>
              )}

              {/* Last Updated */}
              {project.updated_at && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <History className="h-3 w-3 shrink-0" />
                    <span className="text-sm font-medium">Last Updated</span>
                  </div>
                  <span className="text-sm">
                    {new Date(project.updated_at).toLocaleString()}
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
                    {project.id}
                  </code>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    className="text-muted-foreground hover:text-muted-foreground-strong"
                    onClick={async () => {
                      await navigator.clipboard.writeText(project.id);
                      setCopiedId(true);
                      setTimeout(() => setCopiedId(false), 2000);
                    }}
                    aria-label="Copy project ID"
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
                      <TableHead>Version</TableHead>
                      <TableHead>Description</TableHead>
                      <TableHead>Created</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {versions.map((v: RemoteProjectVersion) => (
                      <TableRow key={v.id || v.version_number}>
                        <TableCell>
                          <Badge variant="outline">v{v.version_number}</Badge>
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
                      <TableHead>Version</TableHead>
                      <TableHead>Created</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {tags.map((t: RemoteProjectTag) => (
                      <TableRow key={t.tag}>
                        <TableCell>
                          <Badge variant="outline">{t.tag}</Badge>
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          v{t.version_number}
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
