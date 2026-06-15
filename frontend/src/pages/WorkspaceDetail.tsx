import { useQueryClient } from '@tanstack/react-query';
import {
  ArrowLeft,
  Boxes,
  Calendar,
  Check,
  CircleQuestionMark,
  Copy,
  ExternalLink,
  Fingerprint,
  FolderOpen,
  GitBranch,
  Globe,
  HardDrive,
  History,
  IdCard,
  Loader2,
  Lock,
  Package,
  Pencil,
  Save,
  User,
  Users,
  Users2,
  X,
} from 'lucide-react';
import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { workspacesApi } from '@/api/workspaces';
import { Jobs } from '@/components/jobs/Jobs';
import { PublishButton } from '@/components/publishing/PublishButton';
import { CollaboratorsList } from '@/components/sharing/CollaboratorsList';
import { ShareButton } from '@/components/sharing/ShareButton';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { UserBadge } from '@/components/ui/user-badge';
import { VersionHistory } from '@/components/versions/VersionHistory';
import { PixiTomlEditor } from '@/components/workspace/PixiTomlEditor';
import { useCollaborators } from '@/hooks/useAdmin';
import { usePackages } from '@/hooks/usePackages';
import { usePublications, useUpdatePublication } from '@/hooks/useRegistries';
import { useWorkspace } from '@/hooks/useWorkspaces';
import { buildImportCommand } from '@/lib/registry';
import { capitalize } from '@/lib/utils';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';
import { useWorkspaceNavStore } from '@/store/workspaceNavStore';
import type { Collaborator } from '@/types/models';

const statusColors: Record<string, string> = {
  pending: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
  creating: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  running: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  ready: 'bg-green-500/10 text-green-500 border-green-500/20',
  failed: 'bg-red-500/10 text-red-500 border-red-500/20',
  deleting: 'bg-orange-500/10 text-orange-500 border-orange-500/20',
};

export const WorkspaceDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const consumePendingTab = useWorkspaceNavStore((s) => s.consumePendingTab);
  const wsId = id || '';

  const { data: workspace, isLoading: wsLoading } = useWorkspace(wsId);
  const { data: packages, isLoading: packagesLoading } = usePackages(wsId);
  const { data: collaborators } = useCollaborators(wsId);
  const userCollaborators = collaborators?.filter(
    (c): c is Extract<Collaborator, { kind: 'user' }> => c.kind === 'user',
  );
  const groupCollaborators = collaborators?.filter(
    (c): c is Extract<Collaborator, { kind: 'group' }> => c.kind === 'group',
  );
  const { data: publications, isLoading: publicationsLoading } =
    usePublications(wsId);
  const updatePubMutation = useUpdatePublication();
  const currentUser = useAuthStore((state) => state.user);

  const [activeTab, setActiveTab] = useState(
    () => consumePendingTab() || 'overview',
  );
  const [error, setError] = useState('');
  const [pixiToml, setPixiToml] = useState<string>('');
  const [editedToml, setEditedToml] = useState<string>('');
  const [isEditingToml, setIsEditingToml] = useState(false);
  const [savingToml, setSavingToml] = useState(false);
  const [loadingToml, setLoadingToml] = useState(false);
  const [copiedToml, setCopiedToml] = useState(false);
  const [copiedPull, setCopiedPull] = useState(false);
  const [copiedImportId, setCopiedImportId] = useState<string | null>(null);
  const [copiedId, setCopiedId] = useState(false);
  const [saveInstallJobId, setSaveInstallJobId] = useState<string | null>(null);

  // Determine if this is a local workspace
  const isLocalWs = workspace?.source === 'local';
  // Determine if server is in local mode
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  // User can only share if it's not a local workspace and they are the owner
  const isOwner = workspace?.owner_id === currentUser?.id;

  // Load pixi.toml when switching to that tab
  useEffect(() => {
    if (activeTab === 'toml' && !pixiToml) {
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

  const handleCopyPull = async () => {
    const serverUrl = window.location.origin;
    const cmd = `nebi login ${serverUrl} && nebi pull ${workspace?.name || ''}`;
    await navigator.clipboard.writeText(cmd);
    setCopiedPull(true);
    setTimeout(() => setCopiedPull(false), 2000);
  };

  const handleCopyImport = async (pub: {
    registry_url: string;
    registry_namespace: string;
    repository: string;
    tag: string;
    id: string;
  }) => {
    const repo = pub.registry_namespace
      ? `${pub.registry_namespace}/${pub.repository}`
      : pub.repository;
    const cmd = buildImportCommand(pub.registry_url, repo, pub.tag);
    await navigator.clipboard.writeText(cmd);
    setCopiedImportId(pub.id);
    setTimeout(() => setCopiedImportId(null), 2000);
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
        <Button
          variant="ghost"
          size="icon"
          onClick={() => navigate('/workspaces')}
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <h1 className="text-3xl font-bold">{workspace.name}</h1>
          <p className="text-muted-foreground">
            Workspace details and packages
          </p>
        </div>
        <div className="flex items-center gap-2">
          {isLocalWs && (
            <Badge
              variant="outline"
              className="bg-cyan-500/10 text-cyan-500 border-cyan-500/20 gap-1"
            >
              <HardDrive className="h-3 w-3" />
              Local
            </Badge>
          )}
          <Badge
            className={
              statusColors[workspace.status] ||
              'bg-zinc-500/10 text-zinc-500 border-zinc-500/20'
            }
          >
            {capitalize(workspace.status)}
          </Badge>
          {!isLocalWs && (
            <Button
              variant="outline"
              size="sm"
              className="gap-2"
              onClick={handleCopyPull}
            >
              {copiedPull ? (
                <>
                  <Check className="h-4 w-4" />
                  Copied
                </>
              ) : (
                <>
                  <Copy className="h-4 w-4" />
                  nebi pull
                </>
              )}
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            className="gap-2"
            onClick={async () => {
              if (!pixiToml) {
                setLoadingToml(true);
                try {
                  const { content } = await workspacesApi.getPixiToml(wsId);
                  setPixiToml(content);
                  setEditedToml(content);
                } catch {
                  setError('Failed to load pixi.toml');
                  return;
                } finally {
                  setLoadingToml(false);
                }
              } else {
                setEditedToml(pixiToml);
              }
              setActiveTab('toml');
              setIsEditingToml(true);
            }}
          >
            <Pencil className="h-4 w-4" />
            Edit
          </Button>
          <PublishButton
            environmentId={wsId}
            environmentName={workspace.name}
            environmentStatus={workspace.status}
          />
          {!isLocalWs && isOwner && <ShareButton environmentId={wsId} />}
        </div>
      </div>

      <Tabs
        value={activeTab}
        onValueChange={(tab) => {
          setActiveTab(tab);
          setError('');
        }}
      >
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="toml">Configuration</TabsTrigger>
          <TabsTrigger value="versions">Versions</TabsTrigger>
          <TabsTrigger value="packages">Packages</TabsTrigger>
          <TabsTrigger value="jobs">Jobs</TabsTrigger>
          <TabsTrigger value="publications">
            Publications ({publications?.length || 0})
          </TabsTrigger>
          {!isLocalWs && !isLocalMode && (
            <TabsTrigger value="collaborators">
              Collaborators ({collaborators?.length || 0})
            </TabsTrigger>
          )}
        </TabsList>

        <TabsContent value="overview" className="px-1">
          <div className="space-y-4 my-4">
            <h2 className="text-2xl font-bold mb-0">Overview</h2>
            <p className="text-muted-foreground text-sm mt-2">
              View details for the active version of this workspace
            </p>
          </div>
          <div>
            <div>
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <IdCard className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">Workspace Name</span>
                </div>
                <span className="text-sm">{workspace.name}</span>
              </div>

              {/* Owner */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <User className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">Owner</span>
                </div>
                <div className="flex items-center gap-2">
                  <UserBadge
                    username={
                      workspace.owner?.username ||
                      (isOwner ? currentUser?.username || 'You' : 'Unknown')
                    }
                  />
                </div>
              </div>

              {/* Status */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <CircleQuestionMark className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">Status</span>
                </div>
                <div>
                  <Badge
                    className={
                      statusColors[workspace.status] ||
                      'bg-zinc-500/10 text-zinc-500 border-zinc-500/20'
                    }
                  >
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
                  {workspace.package_manager}
                </code>
              </div>

              {/* Path (local workspaces only) */}
              {isLocalWs && workspace.path && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <FolderOpen className="h-3 w-3 shrink-0" />
                    <span className="text-sm font-medium">Path</span>
                  </div>
                  <code
                    className="text-sm font-mono truncate max-w-md"
                    title={workspace.path}
                  >
                    {workspace.path}
                  </code>
                </div>
              )}

              {/* Origin (local workspaces only) */}
              {isLocalWs && workspace.origin_name && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <GitBranch className="h-3 w-3 shrink-0" />
                    <span className="text-sm font-medium">Origin</span>
                  </div>
                  <span className="text-sm">
                    {workspace.origin_name}
                    {workspace.origin_tag && `:${workspace.origin_tag}`}
                    {workspace.origin_action && ` (${workspace.origin_action})`}
                  </span>
                </div>
              )}

              {/* Size */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <HardDrive className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">Size</span>
                </div>
                <span className="text-sm">
                  {workspace.size_formatted || 'Calculating...'}
                </span>
              </div>

              {/* Packages — links to packages tab */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <button
                  className="flex items-center gap-1.5 text-muted-foreground hover:text-foreground transition-colors text-left"
                  onClick={() => setActiveTab('packages')}
                >
                  <Boxes className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium underline decoration-dotted underline-offset-2">
                    Packages
                  </span>
                </button>
                <span className="text-sm">
                  {packages?.length || 0} installed
                </span>
              </div>

              {/* Collaborators — links to collaborators tab (non-local, non-local-mode workspaces) */}
              {!isLocalWs && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  {!isLocalMode ? (
                    <button
                      className="flex items-center gap-1.5 text-muted-foreground hover:text-foreground transition-colors text-left"
                      onClick={() => setActiveTab('collaborators')}
                    >
                      <Users className="h-3 w-3 shrink-0" />
                      <span className="text-sm font-medium underline decoration-dotted underline-offset-2">
                        Collaborators ({userCollaborators?.length || 0})
                      </span>
                    </button>
                  ) : (
                    <div className="flex items-center gap-1.5 text-muted-foreground">
                      <Users className="h-3 w-3 shrink-0" />
                      <span className="text-sm font-medium">
                        Collaborators ({userCollaborators?.length || 0})
                      </span>
                    </div>
                  )}
                  <div className="flex flex-wrap gap-1.5">
                    {userCollaborators?.slice(0, 3).map((c) => (
                      <UserBadge key={c.user_id} username={c.username} />
                    ))}
                    {(userCollaborators?.length || 0) > 3 && (
                      <span className="text-xs text-muted-foreground self-center">
                        +{(userCollaborators?.length || 0) - 3} more
                      </span>
                    )}
                  </div>
                </div>
              )}

              {/* Groups — only shown when the workspace has any group shares */}
              {!isLocalWs && (groupCollaborators?.length || 0) > 0 && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  {!isLocalMode ? (
                    <button
                      className="flex items-center gap-1.5 text-muted-foreground hover:text-foreground transition-colors text-left"
                      onClick={() => setActiveTab('collaborators')}
                    >
                      <Users2 className="h-3 w-3 shrink-0" />
                      <span className="text-sm font-medium underline decoration-dotted underline-offset-2">
                        Groups ({groupCollaborators?.length || 0})
                      </span>
                    </button>
                  ) : (
                    <div className="flex items-center gap-1.5 text-muted-foreground">
                      <Users2 className="h-3 w-3 shrink-0" />
                      <span className="text-sm font-medium">
                        Groups ({groupCollaborators?.length || 0})
                      </span>
                    </div>
                  )}
                  <div className="flex flex-wrap gap-1.5">
                    {groupCollaborators?.slice(0, 3).map((g) => (
                      <Badge
                        key={g.group_id}
                        variant="outline"
                        className={
                          g.source === 'oidc'
                            ? 'border-blue-500/40 text-blue-500'
                            : ''
                        }
                      >
                        {g.name}
                      </Badge>
                    ))}
                    {(groupCollaborators?.length || 0) > 3 && (
                      <span className="text-xs text-muted-foreground self-center">
                        +{(groupCollaborators?.length || 0) - 3} more
                      </span>
                    )}
                  </div>
                </div>
              )}

              {/* Created */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <Calendar className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">Created</span>
                </div>
                <span className="text-sm">
                  {new Date(workspace.created_at).toLocaleString()}
                </span>
              </div>

              {/* Last Updated */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <History className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">Last Updated</span>
                </div>
                <span className="text-sm">
                  {new Date(workspace.updated_at).toLocaleString()}
                </span>
              </div>

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
        </TabsContent>

        <TabsContent value="packages" className="px-1">
          <div className="space-y-4 my-3">
            <div className="flex justify-between items-center  mb-0">
              <h2 className="text-2xl font-bold">Packages</h2>
            </div>
            <div>
              <p className="text-muted-foreground text-sm mt-1">
                Packages installed in the current version. Edit the
                configuration to change packages.
              </p>
            </div>

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
                <CardContent className="p-0">
                  <div>
                    <table className="w-full">
                      <thead className="border-b bg-muted/70">
                        <tr>
                          <th className="text-left p-4 font-medium">Package</th>
                          <th className="text-left p-4 font-medium">
                            Installed Version
                          </th>
                        </tr>
                      </thead>
                      <tbody className="divide-y">
                        {packages?.length === 0 ? (
                          <tr>
                            <td
                              colSpan={2}
                              className="p-8 text-center text-muted-foreground"
                            >
                              No packages installed
                            </td>
                          </tr>
                        ) : (
                          packages?.map((pkg) => (
                            <tr key={pkg.id} className="hover:bg-muted/50">
                              <td className="p-4">
                                <div className="flex items-center gap-2">
                                  <Package className="h-4 w-4 text-muted-foreground" />
                                  <span className="font-medium">
                                    {pkg.name}
                                  </span>
                                </div>
                              </td>
                              <td className="p-4 text-muted-foreground font-mono text-sm">
                                {pkg.version || '-'}
                              </td>
                            </tr>
                          ))
                        )}
                      </tbody>
                    </table>
                  </div>
                </CardContent>
              </Card>
            )}
          </div>
        </TabsContent>

        <TabsContent value="toml" className="px-1">
          <div className="space-y-4 my-4">
            <h2 className="text-2xl font-bold mb-0">Configuration</h2>
            <p className="text-muted-foreground text-sm mt-2">
              Configuration defined in the pixi.toml for the current version
            </p>
          </div>
          {saveInstallJobId && (
            <div className="mb-4 rounded-md border border-blue-500/20 bg-blue-500/10 px-4 py-3 text-sm text-blue-700">
              <div className="flex items-center justify-between gap-3">
                <span>
                  Save complete. Install job started (ID: {saveInstallJobId}).
                </span>
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setActiveTab('jobs')}
                  >
                    View logs
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-blue-700 hover:bg-blue-500/10 hover:text-blue-700"
                    onClick={() => setSaveInstallJobId(null)}
                    aria-label="Dismiss notification"
                  >
                    <X className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </div>
          )}
          <div className="flex items-center justify-between pb-2">
            <div className="flex items-center gap-2">
              {pixiToml && !isEditingToml && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setEditedToml(pixiToml);
                    setSaveInstallJobId(null);
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
                      setError('');
                      setSaveInstallJobId(null);
                      setSavingToml(true);
                      try {
                        await workspacesApi.savePixiToml(wsId, editedToml);
                        const job = await workspacesApi.solveWorkspace(wsId);
                        setPixiToml(editedToml);
                        setIsEditingToml(false);
                        setSaveInstallJobId(job.id);
                        await Promise.all([
                          queryClient.invalidateQueries({
                            queryKey: ['workspaces'],
                          }),
                          queryClient.invalidateQueries({
                            queryKey: ['workspaces', wsId],
                          }),
                          queryClient.invalidateQueries({
                            queryKey: ['jobs'],
                          }),
                        ]);
                      } catch {
                        setError('Failed to save and install pixi.toml');
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
                    Save & Install
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
          {loadingToml ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          ) : isEditingToml ? (
            <PixiTomlEditor
              tomlValue={editedToml}
              onTomlChange={setEditedToml}
              workspaceName={workspace.name}
              onReloadToml={async () => {
                const { content } = await workspacesApi.getPixiToml(wsId);
                return content;
              }}
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
        </TabsContent>

        <TabsContent value="versions" className="px-1">
          <VersionHistory
            environmentId={wsId}
            environmentStatus={workspace.status}
          />
        </TabsContent>

        <TabsContent value="jobs" className="px-1">
          <Jobs workspaceId={wsId} />
        </TabsContent>

        <TabsContent value="publications" className="px-1">
          <div className="space-y-4 my-4">
            <h2 className="text-2xl font-bold mb-0">Publications</h2>
            <p className="text-muted-foreground text-sm mt-2">
              View all publications for this workspace
            </p>
          </div>
          {publicationsLoading ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          ) : publications && publications.length > 0 ? (
            <div className="space-y-3">
              {publications.map((pub) => (
                <div key={pub.id} className="p-4 rounded-lg border">
                  <div className="flex items-start justify-between">
                    <div className="flex-1 space-y-2">
                      <div className="flex items-center gap-2">
                        <a
                          href={`https://${pub.registry_url.replace(/^https?:\/\//, '').replace(/\/$/, '')}/repository/${pub.registry_namespace ? `${pub.registry_namespace}/` : ''}${pub.repository}?tab=tags`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="font-medium text-lg hover:underline text-primary flex items-center gap-1"
                        >
                          {pub.repository}:{pub.tag}
                          <ExternalLink className="h-4 w-4" />
                        </a>
                        {pub.is_public ? (
                          <Badge className="bg-green-500/10 text-green-600 border-green-500/20">
                            <Globe className="mr-1 h-3 w-3" />
                            Public
                          </Badge>
                        ) : (
                          <Badge className="bg-orange-500/10 text-orange-600 border-orange-500/20">
                            <Lock className="mr-1 h-3 w-3" />
                            Private
                          </Badge>
                        )}
                      </div>
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-sm">
                        <div>
                          <span className="text-muted-foreground">
                            Registry:
                          </span>
                          <span className="ml-2 font-medium">
                            {pub.registry_name}
                          </span>
                        </div>
                        <div>
                          <span className="text-muted-foreground">URL:</span>
                          <span className="ml-2 font-mono text-xs">
                            {pub.registry_url}
                          </span>
                        </div>
                        <div>
                          <span className="text-muted-foreground">
                            Published by:
                          </span>
                          <span className="ml-2 font-medium">
                            {pub.published_by}
                          </span>
                        </div>
                        <div>
                          <span className="text-muted-foreground">
                            Published:
                          </span>
                          <span className="ml-2">
                            {new Date(pub.published_at).toLocaleString()}
                          </span>
                        </div>
                      </div>
                      <div className="pt-2">
                        <span className="text-muted-foreground text-sm">
                          Digest:
                        </span>
                        <code className="ml-2 text-xs font-mono bg-muted px-2 py-1 rounded">
                          {pub.digest}
                        </code>
                      </div>
                    </div>
                    <div className="flex items-center gap-2 ml-4">
                      <Button
                        variant="outline"
                        size="sm"
                        className="gap-1"
                        onClick={() => handleCopyImport(pub)}
                      >
                        {copiedImportId === pub.id ? (
                          <>
                            <Check className="h-3.5 w-3.5" />
                            Copied
                          </>
                        ) : (
                          <>
                            <Copy className="h-3.5 w-3.5" />
                            nebi import
                          </>
                        )}
                      </Button>
                      <Button
                        variant={pub.is_public ? 'outline' : 'default'}
                        size="sm"
                        className="gap-1"
                        onClick={() =>
                          updatePubMutation.mutate({
                            workspaceId: wsId,
                            pubId: pub.id,
                            isPublic: !pub.is_public,
                          })
                        }
                        disabled={updatePubMutation.isPending}
                      >
                        {pub.is_public ? (
                          <>
                            <Lock className="h-3.5 w-3.5" />
                            Make Private
                          </>
                        ) : (
                          <>
                            <Globe className="h-3.5 w-3.5" />
                            Make Public
                          </>
                        )}
                      </Button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground text-center py-8">
              No publications yet. Click the "Publish" button to publish this
              workspace to an OCI registry.
            </p>
          )}
        </TabsContent>

        {!isLocalWs && !isLocalMode && (
          <TabsContent value="collaborators" className="px-1">
            <div className="space-y-4 my-4">
              <h2 className="text-2xl font-bold mb-0">Collaborators</h2>
              <p className="text-muted-foreground text-sm mt-2">
                View all collaborators for this workspace
              </p>
            </div>
            <CollaboratorsList collaborators={collaborators || []} />
          </TabsContent>
        )}
      </Tabs>
    </div>
  );
};
