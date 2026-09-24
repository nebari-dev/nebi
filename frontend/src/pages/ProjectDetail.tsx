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
import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { projectsApi } from '@/api/projects';
import { Jobs } from '@/components/jobs/Jobs';
import { InstallControls } from '@/components/project/InstallControls';
import { PixiTomlEditor } from '@/components/project/PixiTomlEditor';
import { UseLocallyButton } from '@/components/project/UseLocallyButton';
import { PublishButton } from '@/components/publishing/PublishButton';
import { CollaboratorsList } from '@/components/sharing/CollaboratorsList';
import { ShareButton } from '@/components/sharing/ShareButton';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
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
import { UserBadge } from '@/components/user-badge';
import { VersionHistory } from '@/components/versions/VersionHistory';
import { useCollaborators } from '@/hooks/useAdmin';
import { usePackages } from '@/hooks/usePackages';
import { useProject } from '@/hooks/useProjects';
import { usePublications, useUpdatePublication } from '@/hooks/useRegistries';
import { buildImportCommand } from '@/lib/registry';
import { getInstallStatusColor, getProjectStatusColor } from '@/lib/status';
import { capitalize } from '@/lib/strings';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';
import { useProjectNavStore } from '@/store/projectNavStore';
import type { Collaborator } from '@/types/models';

export const ProjectDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const consumePendingTab = useProjectNavStore((s) => s.consumePendingTab);
  const projectId = id || '';

  const { data: project, isLoading: projectLoading } = useProject(projectId);
  const { data: packages, isLoading: packagesLoading } = usePackages(projectId);
  const { data: collaborators } = useCollaborators(projectId);
  const userCollaborators = collaborators?.filter(
    (c): c is Extract<Collaborator, { kind: 'user' }> => c.kind === 'user',
  );
  const groupCollaborators = collaborators?.filter(
    (c): c is Extract<Collaborator, { kind: 'group' }> => c.kind === 'group',
  );
  const { data: publications, isLoading: publicationsLoading } =
    usePublications(projectId);
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
  const [copiedImportId, setCopiedImportId] = useState<string | null>(null);
  const [copiedId, setCopiedId] = useState(false);
  const [saveInstallJobId, setSaveInstallJobId] = useState<string | null>(null);
  const [envJobNotice, setEnvJobNotice] = useState<{
    id: string;
    type: string;
  } | null>(null);

  // Determine if this is a local project
  const isLocalProject = project?.source === 'local';
  // Determine if server is in local mode
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  // User can only share if it's not a local project and they are the owner
  const isOwner = project?.owner_id === currentUser?.id;
  const canWrite = project?.can_write === true;
  const editingToml = isEditingToml && canWrite;

  const loadPixiToml = useCallback(async () => {
    setLoadingToml(true);
    try {
      const { content } = await projectsApi.getPixiToml(projectId);
      setPixiToml(content);
    } catch {
      setError('Failed to load pixi.toml');
    } finally {
      setLoadingToml(false);
    }
  }, [projectId]);

  // Load pixi.toml when switching to that tab
  useEffect(() => {
    if (activeTab === 'toml' && !pixiToml) {
      void loadPixiToml();
    }
  }, [activeTab, pixiToml, loadPixiToml]);

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

  if (projectLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!project) {
    return <div>Project not found</div>;
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
          <p className="text-muted-foreground">Project details and packages</p>
        </div>
        <div className="flex items-center gap-2">
          {isLocalProject && (
            <Badge
              variant="outline"
              className="bg-cyan-500/10 text-cyan-500 border-cyan-500/20 gap-1"
            >
              <HardDrive className="h-3 w-3" />
              Local
            </Badge>
          )}
          <Badge className={getProjectStatusColor(project.status)}>
            {capitalize(project.status)}
          </Badge>
          {project.install_status && (
            <Badge className={getInstallStatusColor(project.install_status)}>
              {capitalize(project.install_status.replaceAll('_', ' '))}
            </Badge>
          )}
          <InstallControls
            projectId={projectId}
            installStatus={project.install_status}
            onStarted={(job) => setEnvJobNotice({ id: job.id, type: job.type })}
          />
          {!isLocalProject && <UseLocallyButton projectName={project.name} />}
          <Button
            variant="outline"
            size="sm"
            className="gap-2"
            disabled={!canWrite}
            aria-describedby={!canWrite ? 'configuration-read-only' : undefined}
            onClick={async () => {
              if (!pixiToml) {
                setLoadingToml(true);
                try {
                  const { content } = await projectsApi.getPixiToml(projectId);
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
            environmentId={projectId}
            environmentName={project.name}
            environmentStatus={project.status}
          />
          {!isLocalProject && isOwner && (
            <ShareButton environmentId={projectId} />
          )}
        </div>
      </div>

      {!canWrite && (
        <p
          id="configuration-read-only"
          className="text-sm text-muted-foreground"
        >
          You have read-only access. Editing the configuration requires write
          access.
        </p>
      )}

      {envJobNotice && (
        <div className="rounded-md border border-blue-500/20 bg-blue-500/10 px-4 py-3 text-sm text-blue-700">
          <div className="flex items-center justify-between gap-3">
            <span>
              {envJobNotice.type === 'env_uninstall' ? 'Uninstall' : 'Install'}{' '}
              job started (ID: {envJobNotice.id}).
            </span>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setActiveTab('jobs');
                  setEnvJobNotice(null);
                }}
              >
                View logs
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8 text-blue-700 hover:bg-blue-500/10 hover:text-blue-700"
                onClick={() => setEnvJobNotice(null)}
                aria-label="Dismiss notification"
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
      )}

      <Tabs
        value={activeTab}
        onValueChange={(tab) => {
          setActiveTab(tab);
          setError('');
        }}
      >
        <TabsList>
          <TabsTab value="overview">Overview</TabsTab>
          <TabsTab value="toml">Configuration</TabsTab>
          <TabsTab value="versions">Versions</TabsTab>
          <TabsTab value="packages">Packages</TabsTab>
          <TabsTab value="jobs">Jobs</TabsTab>
          <TabsTab value="publications">
            Publications ({publications?.length || 0})
          </TabsTab>
          {!isLocalProject && !isLocalMode && (
            <TabsTab value="collaborators">
              Collaborators ({collaborators?.length || 0})
            </TabsTab>
          )}
        </TabsList>

        <TabsPanel value="overview" className="px-1">
          <div className="space-y-4 my-4">
            <h2 className="text-2xl font-bold mb-0">Overview</h2>
            <p className="text-muted-foreground text-sm mt-2">
              View details for the active version of this project
            </p>
          </div>
          <div>
            <div>
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <IdCard className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">Project Name</span>
                </div>
                <span className="text-sm">{project.name}</span>
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
                      project.owner?.username ||
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
                  <Badge className={getProjectStatusColor(project.status)}>
                    {capitalize(project.status)}
                  </Badge>
                </div>
              </div>

              {/* Path (local projects only) */}
              {isLocalProject && project.path && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <FolderOpen className="h-3 w-3 shrink-0" />
                    <span className="text-sm font-medium">Path</span>
                  </div>
                  <code
                    className="text-sm font-mono truncate max-w-md"
                    title={project.path}
                  >
                    {project.path}
                  </code>
                </div>
              )}

              {/* Origin (local projects only) */}
              {isLocalProject && project.origin_name && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <GitBranch className="h-3 w-3 shrink-0" />
                    <span className="text-sm font-medium">Origin</span>
                  </div>
                  <span className="text-sm">
                    {project.origin_name}
                    {project.origin_tag && `:${project.origin_tag}`}
                    {project.origin_action && ` (${project.origin_action})`}
                  </span>
                </div>
              )}

              {/* Size */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <HardDrive className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">Size</span>
                </div>
                <span className="text-sm">{project.size_formatted || '-'}</span>
              </div>

              {/* Packages — links to packages tab */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <button
                  type="button"
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

              {/* Collaborators — links to collaborators tab (non-local, non-local-mode projects) */}
              {!isLocalProject && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  {!isLocalMode ? (
                    <button
                      type="button"
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

              {/* Groups — only shown when the project has any group shares */}
              {!isLocalProject && (groupCollaborators?.length || 0) > 0 && (
                <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                  {!isLocalMode ? (
                    <button
                      type="button"
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
                  {new Date(project.created_at).toLocaleString()}
                </span>
              </div>

              {/* Last Updated */}
              <div className="grid grid-cols-[220px_1fr] items-center gap-4 py-2.5">
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <History className="h-3 w-3 shrink-0" />
                  <span className="text-sm font-medium">Last Updated</span>
                </div>
                <span className="text-sm">
                  {new Date(project.updated_at).toLocaleString()}
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
                    {project.id}
                  </code>
                  <button
                    type="button"
                    className="p-1 rounded hover:bg-muted transition-colors text-muted-foreground"
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
                  </button>
                </div>
              </div>
            </div>
          </div>
        </TabsPanel>

        <TabsPanel value="packages" className="px-1">
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
              <Table aria-label="Packages">
                <TableHeader>
                  <TableRow>
                    <TableHead>Package</TableHead>
                    <TableHead>Installed Version</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {packages?.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={2}
                        className="p-8 text-center text-muted-foreground"
                      >
                        No packages installed
                      </TableCell>
                    </TableRow>
                  ) : (
                    packages?.map((pkg) => (
                      <TableRow key={pkg.id}>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <Package className="h-4 w-4 text-muted-foreground" />
                            <span className="font-medium">{pkg.name}</span>
                          </div>
                        </TableCell>
                        <TableCell className="text-muted-foreground font-mono">
                          {pkg.version || '-'}
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            )}
          </div>
        </TabsPanel>

        <TabsPanel value="toml" className="px-1">
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
              {pixiToml && !editingToml && (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!canWrite}
                  aria-describedby={
                    !canWrite ? 'configuration-read-only' : undefined
                  }
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
              {editingToml && (
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
                        await projectsApi.savePixiToml(projectId, editedToml);
                        const job = await projectsApi.solveProject(projectId);
                        setPixiToml(editedToml);
                        setIsEditingToml(false);
                        setSaveInstallJobId(job.id);
                        await Promise.all([
                          queryClient.invalidateQueries({
                            queryKey: ['projects'],
                          }),
                          queryClient.invalidateQueries({
                            queryKey: ['projects', projectId],
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
            </div>
          </div>
          {loadingToml ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          ) : editingToml ? (
            <PixiTomlEditor
              tomlValue={editedToml}
              onTomlChange={setEditedToml}
              projectName={project.name}
              onReloadToml={async () => {
                const { content } = await projectsApi.getPixiToml(projectId);
                return content;
              }}
            />
          ) : pixiToml ? (
            <CodeBlock code={pixiToml} className="w-full">
              <CodeBlockBody aria-label="pixi.toml contents" />
            </CodeBlock>
          ) : (
            <div className="text-center py-8 text-muted-foreground">
              Failed to load pixi.toml
            </div>
          )}
        </TabsPanel>

        <TabsPanel value="versions" className="px-1">
          <VersionHistory
            environmentId={projectId}
            environmentStatus={project.status}
          />
        </TabsPanel>

        <TabsPanel value="jobs" className="px-1">
          <Jobs projectId={projectId} />
        </TabsPanel>

        <TabsPanel value="publications" className="px-1">
          <div className="space-y-4 my-4">
            <h2 className="text-2xl font-bold mb-0">Publications</h2>
            <p className="text-muted-foreground text-sm mt-2">
              View all publications for this project
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
                            projectId: projectId,
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
              project to an OCI registry.
            </p>
          )}
        </TabsPanel>

        {!isLocalProject && !isLocalMode && (
          <TabsPanel value="collaborators" className="px-1">
            <div className="space-y-4 my-4">
              <h2 className="text-2xl font-bold mb-0">Collaborators</h2>
              <p className="text-muted-foreground text-sm mt-2">
                View all collaborators for this project
              </p>
            </div>
            <CollaboratorsList collaborators={collaborators || []} />
          </TabsPanel>
        )}
      </Tabs>
    </div>
  );
};
